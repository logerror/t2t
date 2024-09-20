#!/bin/bash

GOOS=linux GOARCH=arm64 go build -o t2t-client-linux-arm64
GOOS=linux GOARCH=amd64 go build -o t2t-client-linux-amd64

GOOS=darwin GOARCH=amd64 go build -o t2t-client-darwin-amd64
GOOS=darwin GOARCH=arm64 go build -o t2t-client-darwin-arm64