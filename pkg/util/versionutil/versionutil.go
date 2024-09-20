package versionutil

import (
	"encoding/json"
	"fmt"
	"github.com/logerror/t2t/pkg/config"
	"io"
	"net/http"

	"github.com/logerror/t2t/pkg/data/version"
)

func GetLatestVersion() (*version.Version, error) {
	resp, err := http.Get(fmt.Sprintf("%s://%s/version", config.Configuration.Server.Schema.Http, config.Configuration.Agent.Host))
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

func GetCurrentClientVersion() string {
	return "1.0.0"
}

func GetCurrentAgentVersion() string {
	return "1.0.0"
}

func GetCurrentServerVersion() string {
	return "1.0.0"
}
