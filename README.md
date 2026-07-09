# t2t (Terminal 2 Terminal)

```
████████╗██████╗ ████████╗
╚══██╔══╝╚════██╗╚══██╔══╝
   ██║    █████╔╝   ██║
   ██║   ██╔═══╝    ██║
   ██║   ███████╗   ██║
   ╚═╝   ╚══════╝   ╚═╝
```

Chinese documentation: [README.zh-CN.md](./README.zh-CN.md)

t2t is a lightweight remote terminal orchestration tool for engineering and operations teams.  
It uses an `Agent + Server + Client/Web` architecture to support multi-host access, collaborative remote sessions, browser terminal operations, and command playback.

Project goal: provide a secure, observable, and easy-to-operate remote terminal platform with minimal deployment overhead.

## Core Capabilities

- **Unified multi-host onboarding**: manage multiple online agents from one control plane
- **CLI + Web access**: connect from command line or browser terminal
- **Collaborative access**: allow multiple users to work on the same target host
- **Terminal Time Machine (MVP)**: review and replay command timeline per session
- **War Room Mode (MVP)**: multi-host split view with command broadcast

## Screenshots

### Dashboard
![t2t dashboard](./resource/img/index.png)

### Web Terminal
![t2t web terminal](./resource/img/terminal.png)

### Terminal Time Machine
![t2t time machine](./resource/img/time-machine.png)

### War Room Mode
![t2t war room](./resource/img/warroom.png)

## Architecture

1. **Agent**
   - Deployed on target hosts
   - Maintains persistent connections between local shell/PTY and server

2. **Server**
   - Works as control plane and traffic hub
   - Handles agent registration, client/web attachment, session lifecycle, and page/API serving

3. **Client / Web**
   - Client: fast command-line attachment
   - Web: visual management, browser terminal, Time Machine, and War Room

Basic flow:
- Agent starts and registers to server
- User initiates a connection from Client or Web
- Server routes traffic by `hostTag + clientId`
- Bidirectional stream forwarding is established for terminal interaction

## Quick Start

### Option A: Docker Compose (recommended)

Requirements:
- Docker
- Docker Compose v2

Run:

```bash
docker compose up -d --build
```

Access:
- Dashboard: `http://localhost:9002`
- Default local account: `admin / admin123`

Useful commands:

```bash
# Follow logs
docker compose logs -f server
docker compose logs -f agent

# Stop and remove containers
docker compose down
```

Notes:
- The compose setup starts one `server` and one `agent` by default.
- `agent` uses `T2T_SERVER_HOST=server:9002` so container-to-container networking works correctly.
- Terminal Time Machine logs are shared through the `t2t_commands` volume (`/tmp/t2t/commands`).

### Option B: Run binaries manually

### 1) Requirements

- Go 1.22+
- Linux/macOS (depending on your deployment target)

### 2) Prepare configuration

Use `cmd/server/config.yaml` as your server configuration template:

```bash
cp cmd/server/config.yaml config.yaml
```

> Default local account example: `admin / admin123`.  
> Please change credentials before production deployment.

### 3) Build binaries

```bash
go build -o build/t2t-server ./cmd/server
go build -o build/t2t-agent ./cmd/agent
go build -o build/t2t-client ./cmd/client
```

### 4) Start server

```bash
./build/t2t-server
```

### 5) Start agent (on target host)

```bash
./build/t2t-agent
```

After startup, agent prints:
- `Host信息` (host tag)
- `授权码` (client ID / auth code)

### 6) Connect to remote terminal

```bash
./build/t2t-client connect -t <Host信息> -c <授权码>
```

Or access Web UI:
- `http://localhost:9002`

## Feature Guide

### Terminal Time Machine (MVP)

Entry: click **终端时光机** on dashboard

Capabilities:
- session list browsing
- command timeline replay (play/pause/speed/seek)
- dangerous command marking

Current MVP details:
- command logs are written to `/tmp/t2t/commands`
- Time Machine reads session data from this directory

### War Room Mode (MVP)

Entry: click **战情室模式** on dashboard

Capabilities:
- select multiple online hosts
- open multiple web terminals in split view
- broadcast one command to all selected terminal panes

## Operations & Security Recommendations

- change default account/password in production
- place server behind reverse proxy and enable HTTPS
- restrict server exposure scope (security groups / firewall)
- set retention and cleanup policies for `/tmp/t2t/commands`

## FAQ

### Why does Time Machine show no sessions?

Please check:
- agent is upgraded to the latest version and restarted
- commands were actually executed in the monitored terminal
- `/tmp/t2t/commands/commands_<hostTag>_<clientId>.json` exists

## Contributing

Issues and PRs are welcome. Recommended check before submission:

```bash
go test ./...
```
