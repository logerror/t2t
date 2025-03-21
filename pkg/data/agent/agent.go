package agent

import "github.com/logerror/t2t/pkg/times"

type Agent struct {
	HostTag       string `json:"hostTag"`
	ClientId      string `json:"clientId"`
	AgentArch     string `json:"agentArch"`
	AgentVersion  string `json:"agentVersion"`
	ClientUser    string `json:"clientUser"`
	ClientVersion string `json:"clientVersion"`

	CreatedAt *times.Timestamp `json:"createdAt"`
	UpdatedAt *times.Timestamp `json:"updatedAt"`
}
