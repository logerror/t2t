package svcconstants

const (
	AgentServerWsSchema   = "ws"
	AgentServerHttpSchema = "http"
	AgentServerHost       = "localhost:9002"
)

const (
	WsT2TAgentTokenHeader     = "X-T2T-Agent-Token"
	WsT2TAgentArchHeader      = "X-T2T-Agent-Arch"
	WsT2TAgentVersionHeader   = "X-T2T-Agent-Version"
	XWsT2TClientUserHeader    = "X-T2T-Client-User"
	XWsT2TClientVersionHeader = "X-T2T-Client-Version"

	JWTSecret = "t2t-secret"
)
