FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
COPY vendor ./vendor
COPY . .

ARG TARGET=server

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/t2t ./cmd/${TARGET}

FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache bash ca-certificates tzdata

COPY --from=builder /out/t2t /app/t2t
COPY cmd/server/config.yaml /app/config.yaml

RUN mkdir -p /tmp/t2t/commands

EXPOSE 9002

ENTRYPOINT ["/app/t2t"]
