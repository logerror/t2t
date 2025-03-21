package versionutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/data/version"
)

func GetLatestVersion() (*version.Version, error) {
	resp, err := http.Get(fmt.Sprintf("%s://%s/version", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var v version.Version
	err = json.Unmarshal(body, &v)
	if err != nil {
		return nil, err
	}

	return &v, nil
}

func GetAgentVersion(hostTag, clientId string) (string, error) {
	resp, err := http.Get(fmt.Sprintf("%s://%s/agent/%s/%s", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost, hostTag, clientId))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var Response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			HostTag       string `json:"hostTag"`
			ClientId      string `json:"clientId"`
			AgentArch     string `json:"agentArch"`
			AgentVersion  string `json:"agentVersion"`
			ClientUser    string `json:"clientUser"`
			ClientVersion string `json:"clientVersion"`
		} `json:"data"`
	}
	err = json.Unmarshal(body, &Response)
	if err != nil {
		return "", err
	}

	return Response.Data.AgentVersion, nil
}

func GetCurrentClientVersion() string {
	return "2.0.0"
}

func GetCurrentAgentVersion() string {
	return "2.0.0"
}

func GetCurrentServerVersion() string {
	return "2.0.0"
}
