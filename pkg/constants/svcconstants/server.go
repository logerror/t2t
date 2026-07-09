package svcconstants

import "os"

const (
	defaultAgentServerWsSchema   = "ws"
	defaultAgentServerHttpSchema = "http"
	defaultAgentServerHost       = "localhost:9002"
)

var (
	AgentServerWsSchema   = getEnv("T2T_SERVER_WS_SCHEMA", defaultAgentServerWsSchema)
	AgentServerHttpSchema = getEnv("T2T_SERVER_HTTP_SCHEMA", defaultAgentServerHttpSchema)
	AgentServerHost       = getEnv("T2T_SERVER_HOST", defaultAgentServerHost)
)

const (
	WsT2TAgentTokenHeader     = "X-T2T-Agent-Token"
	WsT2TAgentArchHeader      = "X-T2T-Agent-Arch"
	WsT2TAgentVersionHeader   = "X-T2T-Agent-Version"
	XWsT2TClientUserHeader    = "X-T2T-Client-User"
	XWsT2TClientVersionHeader = "X-T2T-Client-Version"

	JWTSecret = "t2t-secret"
)

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
