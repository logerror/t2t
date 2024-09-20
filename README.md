# t2t - Terminal to Terminal, access to remote machines by Terminal
   
```
████████╗██████╗ ████████╗
╚══██╔══╝╚════██╗╚══██╔══╝
   ██║    █████╔╝   ██║   
   ██║   ██╔═══╝    ██║   
   ██║   ███████╗   ██║   
   ╚═╝   ╚══════╝   ╚═╝   
                          
```
**t2t**  (Terminal to Terminal)    
**t2t**  is a tool designed to simplify remote host management via the terminal. It allows users to connect easily to multiple remote hosts and manage them concurrently. t2t supports multiple users connecting to the same remote host simultaneously, with a central server managing agent-client connections.

## Features

- **Multi-host management**: Easily connect and manage multiple remote hosts through a unified interface.
- **Concurrent connections**: Support for multiple users connecting to the same remote host simultaneously.
- **Agent-based architecture**: Agents register with the central server, and clients connect to the desired remote hosts via the server.
- **Secure connections**: All connections are secured to ensure safe communication between clients and remote hosts.

## Architecture

1. **Agent**: The agent runs on the remote host and registers itself with the central server.
2. **Server**: Acts as the middle layer that handles requests from clients and routes them to the corresponding agents.
3. **Client**: The client connects to the server, selects the desired remote host (agent), and initiates a terminal session.

The basic flow:
- Agent registers to the server.
- Client sends a request to connect to a remote host through the server.
- Server routes the client to the appropriate agent.
- Multiple clients can connect to a single agent simultaneously.

## Installation

### Prerequisites
- Golang (for building the agent, server, and client)

### Steps

1. Run the server:
   ```
   ./t2t-server
   ```

2. Run the agent on each remote host:
   ```
   ./t2t-agent  will output the host info and code to connect to the server.
   ```
3. Connect to a remote host using the client:
   ```
   ./t2t-client <Host Info>  <Code> // <Host Info> and  <Code>  from step 2
   ```
## Usage

1. Start the server on a central machine.
2. Deploy agents on all remote hosts you wish to manage.
3. Use the client to connect to the server, and specify which agent (remote host) you want to access.
4. Once connected, you will have a terminal session directly with the remote host.

## Contributing

Contributions are welcome! Please feel free to submit a pull request, or open an issue if you find a bug or have a feature request.
