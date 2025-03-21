#!/bin/bash

GOOS=linux GOARCH=arm64 go build -o build/t2t-server-linux-arm64 cmd/server/main.go
GOOS=linux GOARCH=amd64 go build -o build/t2t-server-linux-amd64 cmd/server/main.go
GOOS=darwin GOARCH=amd64 go build -o build/t2t-server-darwin-amd64 cmd/server/main.go
GOOS=darwin GOARCH=arm64 go build -o build/t2t-server-darwin-arm64 cmd/server/main.go

GOOS=linux GOARCH=arm64 go build -o build/t2t-client-linux-arm64 internal/client/client.go
GOOS=linux GOARCH=amd64 go build -o build/t2t-client-linux-amd64 internal/client/client.go
GOOS=darwin GOARCH=amd64 go build -o build/t2t-client-darwin-amd64 internal/client/client_darwin.go
GOOS=darwin GOARCH=arm64 go build -o build/t2t-client-darwin-arm64 internal/client/client_darwin.go

GOOS=linux GOARCH=arm64 go build -o build/t2t-agent-linux-arm64 internal/agent/agent.go
GOOS=linux GOARCH=amd64 go build -o build/t2t-agent-linux-amd64 internal/agent/agent.go
GOOS=darwin GOARCH=amd64 go build -o build/t2t-agent-darwin-amd64 internal/agent/agent_darwin.go
GOOS=darwin GOARCH=arm64 go build -o build/t2t-agent-darwin-arm64 internal/agent/agent_darwin.go

cp t2t-config.yaml build/t2t-config.yaml

