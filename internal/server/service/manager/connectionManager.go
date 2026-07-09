package manager

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/logerror/easylog"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/util/versionutil"
	"go.uber.org/zap"
)

// 定义连接管理结构
type ConnectionManager struct {
	Mutex   sync.RWMutex
	Agents  map[string]*AgentConnection  // key: "{hostTag}-{clientId}"
	Clients map[string]*ClientConnection // key: "{hostTag}-{clientId}"
}

type AgentConnection struct {
	Ws           *websocket.Conn
	uuid         string
	HostTag      string
	ClientId     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	AgentArch    string
	AgentVersion string
}

type ClientConnection struct {
	Ws            *websocket.Conn
	uuid          string
	HostTag       string
	ClientId      string
	CreatedAt     time.Time
	ClientUser    string
	ClientVersion string
}

// 创建新的连接管理器
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		Agents:  make(map[string]*AgentConnection),
		Clients: make(map[string]*ClientConnection),
	}
}

func (cm *ConnectionManager) HandleAgentConnection(ws *websocket.Conn, hostTag, clientId string, r *http.Request) {
	tag := fmt.Sprintf("%s-%s", hostTag, clientId)

	agentConn := &AgentConnection{
		Ws:           ws,
		uuid:         uuid.New().String(),
		HostTag:      hostTag,
		ClientId:     clientId,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		AgentArch:    r.Header.Get(svcconstants.WsT2TAgentArchHeader),
		AgentVersion: r.Header.Get(svcconstants.WsT2TAgentVersionHeader),
	}

	cm.Mutex.Lock()
	oldAgent, exists := cm.Agents[tag]
	cm.Agents[tag] = agentConn
	cm.Mutex.Unlock()

	if exists && oldAgent != nil {
		easylog.Info("Closing old agent connection",
			zap.String("old uuid", oldAgent.uuid),
			zap.String("hostTag", hostTag),
			zap.String("clientId", clientId))
		safeCloseWS(oldAgent.Ws)
	}

	easylog.Info("Agent connected",
		zap.String("agent uuid", agentConn.uuid),
		zap.String("hostTag", hostTag),
		zap.String("clientId", clientId),
		zap.String("agentVersion", agentConn.AgentVersion))

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
	tag := fmt.Sprintf("%s-%s", hostTag, clientId)

	cm.Mutex.RLock()
	agentConn, exists := cm.Agents[tag]
	cm.Mutex.RUnlock()
	if !exists {
		return fmt.Errorf("no agent connection available for %s", tag)
	}

	// 验证 agent 连接是否活跃
	if err := cm.CheckConnection(agentConn.Ws); err != nil {
		if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			cm.RemoveAgent(hostTag, clientId)
		}
		return fmt.Errorf("agent connection is not active: %v", err)
	}

	clientConn := &ClientConnection{
		Ws:            ws,
		uuid:          uuid.New().String(),
		HostTag:       hostTag,
		ClientId:      clientId,
		CreatedAt:     time.Now(),
		ClientUser:    r.Header.Get(svcconstants.XWsT2TClientUserHeader),
		ClientVersion: r.Header.Get(svcconstants.XWsT2TClientVersionHeader),
	}

	// 检查客户端版本
	if clientConn.ClientVersion != versionutil.GetCurrentClientVersion() {
		easylog.Warn("Client version mismatch",
			zap.String("hostTag", hostTag),
			zap.String("clientId", clientId),
			zap.String("clientVersion", clientConn.ClientVersion),
			zap.String("expected version", versionutil.GetCurrentClientVersion()),
		)
	}

	cm.Mutex.Lock()
	oldClient := cm.Clients[tag]
	cm.Clients[tag] = clientConn
	cm.Mutex.Unlock()

	if oldClient != nil && oldClient.Ws != nil {
		easylog.Info("Closing old client connection",
			zap.String("uuid", oldClient.uuid),
			zap.String("hostTag", hostTag),
			zap.String("clientId", clientId))
		safeCloseWS(oldClient.Ws)
	}

	easylog.Info("Client connected",
		zap.String("uuid", clientConn.uuid),
		zap.String("hostTag", hostTag),
		zap.String("clientId", clientId),
		zap.String("clientUser", clientConn.ClientUser),
		zap.String("clientVersion", clientConn.ClientVersion))

	// 设置数据转发
	go cm.SetupDataForwarding(agentConn, clientConn)
	return nil
}

// 设置数据转发
func (cm *ConnectionManager) SetupDataForwarding(agent *AgentConnection, client *ClientConnection) {
	if agent == nil || agent.Ws == nil || client == nil || client.Ws == nil {
		easylog.Error("Invalid connection state",
			zap.Any("agent", agent))
		return
	}

	tag := fmt.Sprintf("%s-%s", agent.HostTag, agent.ClientId)

	// Create error channels to coordinate goroutine cleanup
	clientErrChan := make(chan error, 1)
	agentErrChan := make(chan error, 1)
	done := make(chan struct{})
	var once sync.Once
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
				cm.Mutex.RLock()
				currentAgent := cm.Agents[tag]
				clientWS := client.Ws
				cm.Mutex.RUnlock()

				// 检查连接是否有效
				if currentAgent == nil || currentAgent.Ws == nil || clientWS == nil {
					clientErrChan <- fmt.Errorf("connection no longer valid")
					return
				}

				messageType, data, err := clientWS.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						easylog.Error("Failed to read from client",
							zap.Error(err),
							zap.String("agent uuid", agent.uuid),
							zap.String("client uuid", client.uuid),
							zap.String("tag", tag),
							zap.String("clientUser", client.ClientUser))
					}
					clientErrChan <- err
					once.Do(func() { close(done) })
					return
				}

				if err = currentAgent.Ws.WriteMessage(messageType, data); err != nil {
					easylog.Error("Failed to write to agent",
						zap.Error(err),
						zap.String("agent uuid", agent.uuid),
						zap.String("client uuid", client.uuid),
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
				cm.Mutex.RLock()
				currentAgent := cm.Agents[tag]
				clientWS := client.Ws
				cm.Mutex.RUnlock()

				// 检查连接是否有效
				if currentAgent == nil || currentAgent.Ws == nil || clientWS == nil {
					agentErrChan <- fmt.Errorf("connection no longer valid")
					return
				}

				messageType, data, err := currentAgent.Ws.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						easylog.Error("Failed to read from agent",
							zap.Error(err),
							zap.String("agent uuid", agent.uuid),
							zap.String("client uuid", client.uuid),
							zap.String("tag", tag))
					}
					agentErrChan <- err
					once.Do(func() { close(done) })
					return
				}

				if err = clientWS.WriteMessage(messageType, data); err != nil {
					easylog.Error("Failed to write to client",
						zap.Error(err),
						zap.String("agent uuid", agent.uuid),
						zap.String("client uuid", client.uuid),
						zap.String("tag", tag),
						zap.String("clientUser", client.ClientUser))
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
				zap.String("clientUser", client.ClientUser))
			safeCloseWS(client.Ws)
		case err = <-agentErrChan:
			easylog.Info("Agent connection error detected",
				zap.Error(err),
				zap.String("tag", tag),
				zap.String("clientUser", client.ClientUser))
			safeCloseWS(agent.Ws)
		} // Signal goroutines to stop

		// 清空用户信息
		cm.Mutex.Lock()
		if c, ok := cm.Clients[tag]; ok {
			c.ClientUser = ""
			c.ClientVersion = ""
		}
		cm.Mutex.Unlock()

		easylog.Info("Cleaned up forwarding resources",
			zap.String("tag", tag),
			zap.String("clientUser", client.ClientUser))
	}()
}

func (cm *ConnectionManager) CheckConnection(ws *websocket.Conn) error {
	return ws.WriteMessage(websocket.PingMessage, nil)
}

// 添加新的检查连接方法
func (cm *ConnectionManager) CheckAgentConnection(hostTag, clientId string) error {
	tag := fmt.Sprintf("%s-%s", hostTag, clientId)

	cm.Mutex.RLock()
	conn, exists := cm.Agents[tag]
	cm.Mutex.RUnlock()
	if !exists {
		return fmt.Errorf("agent not found")
	}
	if conn == nil || conn.Ws == nil {
		return fmt.Errorf("agent websocket is nil")
	}

	var err error
	for i := 0; i < 3; i++ {
		err = conn.Ws.WriteMessage(websocket.PingMessage, nil)
		if err == nil {
			return nil
		}
		easylog.Warn("Error sending ping message to agent",
			zap.String("hostTag", hostTag),
			zap.String("clientId", clientId),
			zap.Int("attempt", i+1),
			zap.Error(err))
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

	cm.Mutex.Lock()
	agentConn, hasAgent := cm.Agents[tag]
	if hasAgent {
		delete(cm.Agents, tag)
	}

	clientConn, hasClient := cm.Clients[tag]
	if hasClient {
		delete(cm.Clients, tag)
	}
	cm.Mutex.Unlock()

	if hasAgent && agentConn != nil && agentConn.Ws != nil {
		safeCloseWS(agentConn.Ws)
		easylog.Info("Agent removed", zap.String("hostTag", hostTag), zap.String("clientId", clientId))
	}

	if hasClient && clientConn != nil && clientConn.Ws != nil {
		safeCloseWS(clientConn.Ws)
		easylog.Info("Client removed", zap.String("hostTag", hostTag), zap.String("clientId", clientId), zap.String("clientUuid", clientConn.uuid))
	}
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

func (cm *ConnectionManager) StartHealthCheck(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			cm.Mutex.RLock()
			tags := make([]string, 0, len(cm.Agents))
			for tag := range cm.Agents {
				tags = append(tags, tag)
			}
			cm.Mutex.RUnlock()
			for _, tag := range tags {
				sep := strings.LastIndex(tag, "-")
				if sep == -1 || sep == len(tag)-1 {
					continue
				}
				hostTag := tag[:sep]
				clientId := tag[sep+1:]
				if err := cm.CheckAgentConnection(hostTag, clientId); err != nil {
					easylog.Info("HealthCheck: remove dead agent", zap.String("hostTag", hostTag), zap.String("clientId", clientId))
					cm.RemoveAgent(hostTag, clientId)
				}
			}
		}
	}()
}
