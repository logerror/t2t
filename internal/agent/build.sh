#!/bin/bash

GOOS=linux GOARCH=arm64 go build -o build/t2t-agent-linux-arm64
GOOS=linux GOARCH=amd64 go build -o build/t2t-agent-linux-amd64

GOOS=darwin GOARCH=amd64 go build -o build/t2t-agent-darwin-amd64
GOOS=darwin GOARCH=arm64 go build -o build/t2t-agent-darwin-arm64

