package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/logerror/easylog"
	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/data/agent"
	"github.com/logerror/t2t/pkg/data/common"
	"github.com/logerror/t2t/pkg/data/version"
	"github.com/logerror/t2t/pkg/times"
	"github.com/logerror/t2t/pkg/util/versionutil"
	"go.uber.org/zap"
	xwebsocket "golang.org/x/net/websocket"
)

var (
	Clients      = make(map[string]*xwebsocket.Conn)
	OptionAgents = make(map[string]*agent.Agent)
	mutex        sync.RWMutex
)

// 添加 upgrader 配置
var upgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 或者根据需求实现更严格的检查
	},
}

// 定义连接管理结构
type ConnectionManager struct {
	mutex   sync.RWMutex
	agents  map[string]*AgentConnection  // key: "{hostTag}-{clientId}"
	clients map[string]*ClientConnection // key: "{hostTag}-{clientId}"
}

type AgentConnection struct {
	ws           *websocket.Conn
	hostTag      string
	clientId     string
	createdAt    time.Time
	updatedAt    time.Time
	agentArch    string
	agentVersion string
}

type ClientConnection struct {
	ws            *websocket.Conn
	hostTag       string
	clientId      string
	createdAt     time.Time
	clientUser    string
	clientVersion string
}

// 创建新的连接管理器
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		agents:  make(map[string]*AgentConnection),
		clients: make(map[string]*ClientConnection),
	}
}

// 处理 Agent 连接
func (cm *ConnectionManager) HandleAgentConnection(ws *websocket.Conn, hostTag, clientId string, r *http.Request) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	tag := fmt.Sprintf("%s-%s", hostTag, clientId)

	// 更新 Agent 连接
	agentConn := &AgentConnection{
		ws:           ws,
		hostTag:      hostTag,
		clientId:     clientId,
		createdAt:    time.Now(),
		updatedAt:    time.Now(),
		agentArch:    r.Header.Get(svcconstants.WsT2TAgentArchHeader),
		agentVersion: r.Header.Get(svcconstants.WsT2TAgentVersionHeader),
	}

	// 如果存在旧连接，关闭它
	if oldAgent, exists := cm.agents[tag]; exists {
		oldAgent.ws.Close()
	}

	cm.agents[tag] = agentConn
	easylog.Info("Agent connected",
		zap.String("hostTag", hostTag),
		zap.String("clientId", clientId),
		zap.String("agentVersion", agentConn.agentVersion))

	//select {}
	//// 如果存在客户端连接，更新数据转发
	//if client, exists := cm.clients[tag]; exists {
	//	if err := cm.checkClientConnection(client); err == nil {
	//		go cm.setupDataForwarding(agentConn, client)
	//	}
	//}
}

// 处理 Client 连接
func (cm *ConnectionManager) HandleClientConnection(ws *websocket.Conn, hostTag, clientId string, r *http.Request) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	tag := fmt.Sprintf("%s-%s", hostTag, clientId)
	agentConn, exists := cm.agents[tag]
	if !exists {
		return fmt.Errorf("no agent connection available for %s", tag)
	}

	// 验证 agent 连接是否活跃
	if err := cm.checkConnection(agentConn); err != nil {
		if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			delete(cm.agents, tag)
		}
		return fmt.Errorf("agent connection is not active: %v", err)
	}

	clientConn := &ClientConnection{
		ws:            ws,
		hostTag:       hostTag,
		clientId:      clientId,
		createdAt:     time.Now(),
		clientUser:    r.Header.Get(svcconstants.XWsT2TClientUserHeader),
		clientVersion: r.Header.Get(svcconstants.XWsT2TClientVersionHeader),
	}

	// 检查客户端版本
	if clientConn.clientVersion != versionutil.GetCurrentClientVersion() {
		easylog.Warn("Client version mismatch",
			zap.String("hostTag", hostTag),
			zap.String("clientId", clientId),
			zap.String("clientVersion", clientConn.clientVersion),
			zap.String("expected version", versionutil.GetCurrentClientVersion()),
		)
	}

	// 添加到客户端列表
	cm.clients[tag] = clientConn

	easylog.Info("Client connected",
		zap.String("hostTag", hostTag),
		zap.String("clientId", clientId),
		zap.String("clientUser", clientConn.clientUser),
		zap.String("clientVersion", clientConn.clientVersion))

	// 设置数据转发
	go cm.setupDataForwarding(agentConn, clientConn)
	return nil
}

// 设置数据转发
func (cm *ConnectionManager) setupDataForwarding(agent *AgentConnection, client *ClientConnection) {
	if agent == nil || agent.ws == nil || client == nil || client.ws == nil {
		easylog.Error("Invalid connection state",
			zap.Any("agent", agent))
		return
	}

	tag := fmt.Sprintf("%s-%s", agent.hostTag, agent.clientId)

	// Create error channels to coordinate goroutine cleanup
	clientErrChan := make(chan error, 1)
	agentErrChan := make(chan error, 1)
	done := make(chan struct{})

	// Client -> Agent
	go func() {
		defer func() {
			if r := recover(); r != nil {
				easylog.Error("Panic in client->agent forwarding",
					zap.Any("recover", r),
					zap.String("tag", tag))
				clientErrChan <- fmt.Errorf("panic in forwarding: %v", r)
			}
		}()

		for {
			select {
			case <-done:
				return
			default:
				cm.mutex.RLock()
				currentAgent := cm.agents[tag]
				clientWS := client.ws
				cm.mutex.RUnlock()

				// 检查连接是否有效
				if currentAgent == nil || currentAgent.ws == nil || clientWS == nil {
					clientErrChan <- fmt.Errorf("connection no longer valid")
					return
				}

				messageType, data, err := clientWS.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						easylog.Error("Failed to read from client",
							zap.Error(err),
							zap.String("tag", tag),
							zap.String("clientUser", client.clientUser))
					}
					clientErrChan <- err
					return
				}

				if err = currentAgent.ws.WriteMessage(messageType, data); err != nil {
					easylog.Error("Failed to write to agent",
						zap.Error(err),
						zap.String("tag", tag))
					agentErrChan <- err
					return
				}
			}
		}
	}()

	// Agent -> Client
	go func() {
		defer func() {
			if r := recover(); r != nil {
				easylog.Error("Panic in agent->client forwarding",
					zap.Any("recover", r),
					zap.String("tag", tag))
				agentErrChan <- fmt.Errorf("panic in forwarding: %v", r)
			}
		}()

		for {
			select {
			case <-done:
				return
			default:
				cm.mutex.RLock()
				currentAgent := cm.agents[tag]
				clientWS := client.ws
				cm.mutex.RUnlock()

				// 检查连接是否有效
				if currentAgent == nil || currentAgent.ws == nil || clientWS == nil {
					agentErrChan <- fmt.Errorf("connection no longer valid")
					return
				}

				messageType, data, err := currentAgent.ws.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						easylog.Error("Failed to read from agent",
							zap.Error(err),
							zap.String("tag", tag))
					}
					agentErrChan <- err
					return
				}

				if err = clientWS.WriteMessage(messageType, data); err != nil {
					easylog.Error("Failed to write to client",
						zap.Error(err),
						zap.String("tag", tag),
						zap.String("clientUser", client.clientUser))
					clientErrChan <- err
					return
				}
			}
		}
	}()

	// Wait for any error and cleanup
	go func() {
		var err error
		select {
		case err = <-clientErrChan:
			easylog.Info("Client connection error detected",
				zap.Error(err),
				zap.String("tag", tag),
				zap.String("clientUser", client.clientUser))
			safeCloseWS(client.ws)
			delete(cm.clients, tag)
		case err = <-agentErrChan:
			easylog.Info("Agent connection error detected",
				zap.Error(err),
				zap.String("tag", tag),
				zap.String("clientUser", client.clientUser))
			safeCloseWS(agent.ws)
			delete(cm.clients, tag)
		}

		close(done) // Signal goroutines to stop

		// Ensure proper cleanup of resources
		cm.mutex.Lock()
		defer cm.mutex.Unlock()

		// need close agent or keep alive ?
		//if currentAgent, exists := cm.agents[tag]; exists && currentAgent == agent {
		//	safeCloseWS(currentAgent.ws)
		//	delete(cm.agents, tag)
		//}

		// Remove this client from the clients list
		if _, exists := cm.clients[tag]; exists {
			delete(cm.clients, tag)
		}

		easylog.Info("Cleaned up forwarding resources",
			zap.String("tag", tag),
			zap.String("clientUser", client.clientUser))
	}()
}

// 安全关闭WebSocket连接
func safeCloseWS(ws *websocket.Conn) {
	if ws != nil {
		deadline := time.Now().Add(time.Second)
		// 先尝试发送关闭消息
		err := ws.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			deadline,
		)
		if err != nil {
			easylog.Warn("Failed to send close message", zap.Error(err))
		}
		// 等待一小段时间确保消息发送
		time.Sleep(100 * time.Millisecond)
		// 关闭连接
		ws.Close()
	}
}

// 处理客户端错误
func (cm *ConnectionManager) handleClientError(hostTag, clientId string, client *ClientConnection) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	tag := fmt.Sprintf("%s-%s", hostTag, clientId)
	//clients := cm.clients[tag]

	// 移除断开的客户端
	//for i, c := range clients {
	//	if c == client {
	//		clients[i] = clients[len(clients)-1]
	//		clients = clients[:len(clients)-1]
	//		break
	//	}
	//}
	//
	//if len(clients) == 0 {
	//	delete(cm.clients, tag)
	//} else {
	//	cm.clients[tag] = clients
	//}
	delete(cm.clients, tag)

	easylog.Info("Client disconnected",
		zap.String("hostTag", hostTag),
		zap.String("clientId", clientId),
		zap.String("clientUser", client.clientUser))

	client.ws.Close()
}

// 全局连接管理器实例
var connManager = NewConnectionManager()

func HandleWebSocketV2(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		easylog.Error("Failed to upgrade connection", zap.Error(err))
		return
	}

	pathPattern := regexp.MustCompile(`^/v2/ws/([a-zA-Z0-9_-]+)/([0-9]+)$`)
	matches := pathPattern.FindStringSubmatch(r.URL.Path)

	if matches == nil {
		easylog.Error("Invalid WebSocket path", zap.String("path", r.URL.Path))
		ws.Close()
		return
	}

	hostTag, clientId := matches[1], matches[2]
	connManager.HandleAgentConnection(ws, hostTag, clientId, r)
}

// Client 连接处理函数
func HandleAttachV2(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		jsonError := &common.Response{
			Code:    http.StatusInternalServerError,
			Message: "Failed to upgrade connection",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(jsonError)
		return
	}

	pathPattern := regexp.MustCompile(`^/v2/attach/([a-zA-Z0-9_-]+)/([0-9]+)$`)
	matches := pathPattern.FindStringSubmatch(r.URL.Path)

	if matches == nil {
		// Send JSON error through websocket and close
		jsonError := &common.Response{
			Code:    http.StatusBadRequest,
			Message: "Invalid attach path",
		}
		jsonData, _ := json.Marshal(jsonError)
		ws.WriteMessage(websocket.TextMessage, jsonData)
		ws.Close()
		return
	}

	hostTag, clientId := matches[1], matches[2]
	if err = connManager.HandleClientConnection(ws, hostTag, clientId, r); err != nil {
		jsonError := &common.Response{
			Code:    http.StatusNotFound,
			Message: fmt.Sprintf("Failed to handle client connection: %v", err),
		}
		jsonData, _ := json.Marshal(jsonError)
		ws.WriteMessage(websocket.TextMessage, jsonData)
		ws.Close()
	}
}

func HandleAttach(wsAttach *xwebsocket.Conn) {
	remoteAddr := wsAttach.Request().RemoteAddr
	clientUser := wsAttach.Request().Header.Get(svcconstants.XWsT2TClientUserHeader)
	clientVersion := wsAttach.Request().Header.Get(svcconstants.XWsT2TClientVersionHeader)
	urlPath := wsAttach.Request().URL.Path

	pathPattern := regexp.MustCompile(`^/attach/([a-zA-Z0-9_-]+)/([0-9]+)$`)
	easylog.Info("Handling attach",
		zap.String("urlPath", urlPath),
		zap.String("remoteAddr", remoteAddr),
		zap.String("clientUser", clientUser),
		zap.String("clientVersion", clientVersion),
	)
	matches := pathPattern.FindStringSubmatch(urlPath)
	if matches == nil {
		easylog.Info("Invalid attach request", zap.String("path", urlPath), zap.String("clientUser", clientUser))
		wsAttach.Write([]byte("Invalid attach request"))
		wsAttach.Close()
		return
	}

	hostTag := matches[1]
	clientId := matches[2]
	tag := fmt.Sprintf("%s-%s", hostTag, clientId)
	ws, ok := Clients[tag]
	if !ok {
		wsAttach.Write([]byte("Can't find agent to attach \\n"))
		wsAttach.Close()
		return
	}

	optionAgent := OptionAgents[tag]
	if optionAgent != nil {
		if optionAgent.ClientUser != "" {
			wsAttach.Write([]byte(fmt.Sprintf("Warning: current agent is connected by", optionAgent.ClientUser)))
		}
		optionAgent.ClientUser = clientUser
		optionAgent.ClientVersion = clientVersion
		OptionAgents[tag] = optionAgent
	}

	if clientVersion != versionutil.GetCurrentClientVersion() {
		wsAttach.Write([]byte(fmt.Sprintf("你的Client版本太旧了，快按照文档更新一下吧, 最新版本:", versionutil.GetCurrentClientVersion())))
	}

	logPathDir := fmt.Sprintf("/tmp/server_cache/%s/%s", time.Now().Format("2006_01_02"), hostTag)
	err := os.MkdirAll(logPathDir, 0775)
	if err != nil {
		easylog.Error("Error creating directory:", zap.Error(err))
		return
	}
	file, err := os.OpenFile(path.Join(logPathDir, fmt.Sprintf("%s-%s", clientUser, clientId)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		easylog.Error("failed to open log file", zap.Error(err))
		return
	}
	defer file.Close()

	tr := io.TeeReader(wsAttach, file)

	go func() {
		buf := make([]byte, 1024)
		for {
			n, readErr := tr.Read(buf)
			if readErr != nil {
				easylog.Error("read connection from client err",
					zap.String("clientUser", clientUser),
					zap.String("clientVersion", clientVersion),
					zap.Error(readErr),
				)
				wsAttach.Close()
				break
			}
			_, err = ws.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()

	buf := make([]byte, 1024)
	for {
		n, readErr := ws.Read(buf)
		if readErr != nil {
			easylog.Error("read connection from agent err",
				zap.String("clientUser", clientUser),
				zap.String("hostTag", hostTag),
				zap.String("clientId", clientId),
				zap.Error(readErr),
			)
			RemoveAgent(matches[1], matches[2])
			break
		}
		_, writeErr := wsAttach.Write(buf[:n])
		if writeErr != nil {
			easylog.Error("Error sending data to client", zap.Error(writeErr))
			break
		}
	}
}

func HandleWebSocket(ws *xwebsocket.Conn) {
	urlPath := ws.Request().URL.Path
	agentArch := ws.Request().Header.Get(svcconstants.WsT2TAgentArchHeader)
	agentVersion := ws.Request().Header.Get(svcconstants.WsT2TAgentVersionHeader)
	easylog.Info("Start Handling connection",
		zap.String("urlPath", urlPath),
		zap.String("agentArch", agentArch),
		zap.String("agentVersion", agentVersion),
	)
	pathPattern := regexp.MustCompile(`^/ws/([a-zA-Z0-9_-]+)/([0-9]+)$`)

	matches := pathPattern.FindStringSubmatch(urlPath)
	if matches == nil {
		easylog.Info("Invalid path", zap.String("path", urlPath))
		ws.Close()
		return
	}

	if len(matches) >= 2 {
		RemoveAgent(matches[1], matches[2])
	}

	tag := fmt.Sprintf("%s-%s", matches[1], matches[2])
	AddAgent(matches[1], matches[2], ws)
	OptionAgents[tag] = &agent.Agent{
		HostTag:      matches[1],
		ClientId:     matches[2],
		AgentArch:    agentArch,
		AgentVersion: agentVersion,
		CreatedAt:    times.FromTimep(times.Nowp()),
		UpdatedAt:    times.FromTimep(times.Nowp()),
	}
	select {}
}

func ListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentList := make([]agent.Agent, 0, len(connManager.agents))
	for _, conn := range connManager.agents {
		var clientUser, clientVersion string
		if client, exist := connManager.clients[fmt.Sprintf("%s-%s", conn.hostTag, conn.clientId)]; exist {
			clientUser = client.clientUser
			clientVersion = client.clientVersion
		}

		agentList = append(agentList, agent.Agent{
			HostTag:       conn.hostTag,
			ClientId:      conn.clientId,
			CreatedAt:     times.FromTimep(&conn.createdAt),
			UpdatedAt:     times.FromTimep(&conn.updatedAt),
			AgentArch:     conn.agentArch,
			AgentVersion:  conn.agentVersion,
			ClientUser:    clientUser,
			ClientVersion: clientVersion,
		})
	}

	for _, conn := range OptionAgents {
		agentList = append(agentList, *conn)
	}

	sort.Slice(agentList, func(i, j int) bool {
		return agentList[i].CreatedAt.String() > agentList[j].CreatedAt.String()
	})

	jsonData, err := json.Marshal(agentList)
	if err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}

func AgentOption(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		pathPattern := regexp.MustCompile(`^/agent/([a-zA-Z0-9_-]+)/([0-9]+)$`)
		matches := pathPattern.FindStringSubmatch(r.URL.Path)
		if matches == nil {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		hostTag, clientId := matches[1], matches[2]
		connManager.RemoveAgent(hostTag, clientId)

		response := &common.Response{
			Code:    http.StatusOK,
			Message: "Agent removed successfully",
		}

		jsonData, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
		return
	}

	if r.Method == http.MethodGet {
		pathPattern := regexp.MustCompile(`^/agent/([a-zA-Z0-9_-]+)/([0-9]+)$`)
		matches := pathPattern.FindStringSubmatch(r.URL.Path)
		if matches == nil {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		hostTag, clientId := matches[1], matches[2]
		tag := fmt.Sprintf("%s-%s", hostTag, clientId)
		var optionAgent agent.Agent
		v2Agent, exist := connManager.agents[tag]
		if exist {
			optionAgent = agent.Agent{
				HostTag:      v2Agent.hostTag,
				ClientId:     v2Agent.clientId,
				AgentArch:    v2Agent.agentArch,
				AgentVersion: v2Agent.agentVersion,
			}
		}

		v1Agent, exist := OptionAgents[fmt.Sprintf("%s-%s", hostTag, clientId)]
		if exist {
			optionAgent = *v1Agent
		}

		response := &common.Response{
			Code: http.StatusOK,
			Data: optionAgent,
		}

		jsonData, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

}

func StableVersion(w http.ResponseWriter, r *http.Request) {
	jsonData, err := json.Marshal(&version.Version{
		Agent:  versionutil.GetCurrentAgentVersion(),
		Client: versionutil.GetCurrentClientVersion(),
		Server: versionutil.GetCurrentServerVersion(),
	})
	if err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}

	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func CheckConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathPattern := regexp.MustCompile(`^/check/([a-zA-Z0-9_-]+)/([0-9]+)$`)
	matches := pathPattern.FindStringSubmatch(r.URL.Path)
	if matches == nil || len(matches) != 3 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	hostTag, clientId := matches[1], matches[2]
	deletedAgents := []string{}

	available := false
	if err := connManager.CheckAgentConnection(hostTag, clientId); err != nil {
		tag := fmt.Sprintf("%s-%s", hostTag, clientId)
		deletedAgents = append(deletedAgents, tag)
		connManager.RemoveAgent(hostTag, clientId)

	}
	available = true

	easylog.Info("Checking connection status",
		zap.String("hostTag", hostTag),
		zap.String("clientId", clientId),
		zap.Bool("available", available))

	// 返回 JSON 响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hostTag":   hostTag,
		"clientId":  clientId,
		"available": available,
	})
}

func CheckConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deletedAgents := []string{}
	agents := connManager.agents
	for tag, currentAgent := range agents {
		if err := connManager.CheckAgentConnection(currentAgent.hostTag, currentAgent.clientId); err != nil {
			deletedAgents = append(deletedAgents, tag)
			connManager.RemoveAgent(currentAgent.hostTag, currentAgent.clientId)
		}
	}

	for key, optionAgent := range OptionAgents {
		easylog.Info("Check agent connection", zap.String("Host", key), zap.String("ClientUser", optionAgent.ClientUser))
		if optionAgent.ClientUser != "" {
			continue
		}
		conn, ok := Clients[key]
		if !ok {
			continue
		}

		var checkErr error
		for i := 0; i < 3; i++ {
			_, checkErr = conn.Write([]byte(""))
			if checkErr != nil {
				easylog.Warn("Error sending ping message to agent",
					zap.String("Host", key),
					zap.Int("attempt", i),
					zap.Error(checkErr))
				continue
			} else {
				break
			}
		}

		if checkErr != nil {
			RemoveAgent(optionAgent.HostTag, optionAgent.ClientId)
			deletedAgents = append(deletedAgents, key)
		}
	}

	jsonData, err := json.Marshal(deletedAgents)
	if err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}

func AddAgent(hostTag, clientId string, ws *xwebsocket.Conn) {
	mutex.Lock()
	defer mutex.Unlock()
	tag := fmt.Sprintf("%s-%s", hostTag, clientId)
	Clients[tag] = ws
}

func RemoveAgent(hostTag, clientId string) {
	mutex.Lock()
	defer mutex.Unlock()
	tag := fmt.Sprintf("%s-%s", hostTag, clientId)
	ws, ok := Clients[tag]
	if ok {
		ws.Close()
		easylog.Info("Remove agent connection", zap.String("hostTag", hostTag), zap.String("clientId", clientId))
	}
	delete(Clients, tag)
	delete(OptionAgents, tag)
}

func (cm *ConnectionManager) checkConnection(agent *AgentConnection) error {
	return agent.ws.WriteMessage(websocket.PingMessage, nil)
}

func (cm *ConnectionManager) checkClientConnection(client *ClientConnection) error {
	return client.ws.WriteMessage(websocket.PingMessage, nil)
}

// 添加新的检查连接方法
func (cm *ConnectionManager) CheckAgentConnection(hostTag, clientId string) error {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	tag := fmt.Sprintf("%s-%s", hostTag, clientId)
	conn, exists := cm.agents[tag]
	if !exists {
		return fmt.Errorf("agent not found")
	}

	var err error
	// 尝试发送 ping 消息
	for i := 0; i < 3; i++ {
		err = conn.ws.WriteMessage(websocket.PingMessage, nil)
		if err != nil {
			easylog.Warn("Error sending ping message to agent",
				zap.String("hostTag", hostTag),
				zap.String("clientId", clientId),
				zap.Int("attempt", i),
				zap.Error(err))
			continue
		}
	}

	if err != nil {
		easylog.Warn("remove agent connection", zap.String("hostTag", hostTag), zap.String("clientId", clientId))
		cm.RemoveAgent(hostTag, clientId)
	}
	return err
}

// 添加 RemoveAgent 方法到 ConnectionManager
func (cm *ConnectionManager) RemoveAgent(hostTag, clientId string) {
	tag := fmt.Sprintf("%s-%s", hostTag, clientId)
	if conn, exists := cm.agents[tag]; exists {
		delete(cm.agents, tag)
		err := conn.ws.Close()
		if err != nil {
			easylog.Info("failed to close agent ws", zap.String("hostTag", hostTag), zap.String("clientId", clientId))
		}
	}

	// 关闭所有相关的客户端连接
	if c, exists := cm.clients[tag]; exists {
		delete(cm.clients, tag)
		err := c.ws.Close()
		if err != nil {
			easylog.Info("failed to close client ws", zap.String("hostTag", hostTag), zap.String("clientId", clientId))
		}
	}

	easylog.Info("Agent removed", zap.String("hostTag", hostTag), zap.String("clientId", clientId))
}
