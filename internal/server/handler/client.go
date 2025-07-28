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

	"github.com/logerror/easylog"

	"github.com/logerror/t2t/pkg/util/apiutil"
	"github.com/logerror/t2t/pkg/util/authutil"

	"github.com/logerror/t2t/internal/server/service/manager"

	"github.com/gorilla/websocket"
	"github.com/logerror/t2t/pkg/config"
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

// 处理 Agent 连接

// 全局连接管理器实例
var connManager = manager.NewConnectionManager()

func init() {
	connManager.StartHealthCheck(30 * time.Second)
}

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

	ws.SetPingHandler(func(appData string) error {
		return ws.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(time.Second))
	})

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
	// 新增：从token解析用户名
	token := wsAttach.Request().Header.Get("Authorization")
	if token == "" {
		cookie, err := wsAttach.Request().Cookie("token")
		if err == nil {
			token = cookie.Value
		}
	}
	if token != "" {
		claims, err := authutil.VerifyToken(token)
		if err == nil && claims != nil && claims.Subject != "" {
			clientUser = claims.Subject
		}
	}
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

func Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input common.LoginInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		apiutil.RespondJSON(w, r, http.StatusBadRequest, &common.LoginOutput{Error: err.Error()})
		return
	}

	mode := config.Configuration.Auth.Mode
	if mode == "ldap" {
		valid := false
		for _, user := range config.Configuration.Auth.Users {
			if input.User == user.Username && input.Password == user.Password {
				valid = true
				break
			}
		}

		if !valid {
			ldapUrl := config.Configuration.Auth.LDAPURL
			ldapBaseDN := config.Configuration.Auth.LDAPBaseDN
			ldapServiceUser := config.Configuration.Auth.LDAPServiceUser
			ldapServicePass := config.Configuration.Auth.LDAPServicePass
			pass, err := authutil.AuthenticateLDAP(ldapUrl, ldapBaseDN, ldapServiceUser, ldapServicePass, input.User, input.Password)
			if err != nil {
				apiutil.RespondJSON(w, r, http.StatusBadRequest, &common.LoginOutput{Error: err.Error()})
				return
			}
			if !pass {
				apiutil.RespondJSON(w, r, http.StatusUnauthorized, &common.LoginOutput{Error: "Invalid username or password"})
				return
			}
		}

	} else {
		valid := false
		for _, user := range config.Configuration.Auth.Users {
			if input.User == user.Username && input.Password == user.Password {
				valid = true
				break
			}
		}
		if !valid {
			apiutil.RespondJSON(w, r, http.StatusUnauthorized, &common.LoginOutput{Error: "Invalid username or password"})
			return
		}
	}
	token, err := authutil.GenToken(input.User)
	if err != nil {
		apiutil.RespondJSON(w, r, http.StatusInternalServerError, &common.LoginOutput{Error: "jwt: " + err.Error()})
		return
	}
	apiutil.RespondJSON(w, r, http.StatusOK, &common.LoginOutput{Token: token})
}

func ListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentList := make([]agent.Agent, 0, len(connManager.Agents))
	for _, conn := range connManager.Agents {
		var clientUser, clientVersion string
		if client, exist := connManager.Clients[fmt.Sprintf("%s-%s", conn.HostTag, conn.ClientId)]; exist {
			clientUser = client.ClientUser
			clientVersion = client.ClientVersion
		}

		agentList = append(agentList, agent.Agent{
			HostTag:       conn.HostTag,
			ClientId:      conn.ClientId,
			CreatedAt:     times.FromTimep(&conn.CreatedAt),
			UpdatedAt:     times.FromTimep(&conn.UpdatedAt),
			AgentArch:     conn.AgentArch,
			AgentVersion:  conn.AgentVersion,
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
		v2Agent, exist := connManager.Agents[tag]
		if exist {
			optionAgent = agent.Agent{
				HostTag:      v2Agent.HostTag,
				ClientId:     v2Agent.ClientId,
				AgentArch:    v2Agent.AgentArch,
				AgentVersion: v2Agent.AgentVersion,
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
	agents := connManager.Agents
	for tag, currentAgent := range agents {
		if err := connManager.CheckAgentConnection(currentAgent.HostTag, currentAgent.ClientId); err != nil {
			deletedAgents = append(deletedAgents, tag)
			connManager.RemoveAgent(currentAgent.HostTag, currentAgent.ClientId)
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
