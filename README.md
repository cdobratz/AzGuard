# Agent CLI

A cross-platform CLI tool for software development and multi-cloud cost management, designed for Microsoft environments (PowerShell, Bash, Azure CLI).

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8.svg)

## Features

### Cloud Cost Management
- **Multi-cloud support**: Azure, AWS, GCP
- **Cost tracking**: Current costs, historical trends, forecasting
- **Budget alerts**: Set thresholds and get notified
- **Reports**: Generate JSON/CSV reports

### Software Development
- **Code generation**: AI-powered code generation using Ollama or Anthropic Claude
- **Code review**: AI-powered code analysis
- **Test execution**: Run tests for multiple languages
- **Shell integration**: Execute PowerShell, Bash, Azure CLI commands

### API Server
- **REST API**: HTTP endpoints for all CLI commands
- **Multi-cloud**: Unified API for Azure, AWS, GCP costs

## Installation

### Pre-built Binaries

Download the latest release from [GitHub Releases](https://github.com/yourusername/agent/releases):

```bash
# Windows
curl -L -o agent.exe https://github.com/yourusername/agent/releases/latest/download/agent-windows-amd64.exe

# Linux
curl -L -o agent https://github.com/yourusername/agent/releases/latest/download/agent-linux-amd64

# macOS
curl -L -o agent https://github.com/yourusername/agent/releases/latest/download/Agent-darwin-amd64
```

### Package Managers

```powershell
# Scoop (Windows)
scoop bucket add extras
scoop install agent

# Chocolatey (Windows)
choco install agent

# Homebrew (macOS/Linux)
brew install agent
```

### Build from Source

```bash
git clone https://github.com/yourusername/agent.git
cd agent
go build -o agent ./cmd/agent
go build -o agent-api ./cmd/api
```

## Quick Start

### 1. Configure Azure

```bash
# Set subscription ID
agent config set azure.subscription_id YOUR_SUBSCRIPTION_ID

# Or use Azure CLI auth (default)
az login
```

### 2. Check Cloud Costs

```bash
# Current month costs
agent cost current

# Cost history
agent cost history

# Trend analysis
agent cost trend
```

### 3. Set Budget Alerts

```bash
# Create alert
agent cost alert add monthly-budget 100

# Check alerts
agent cost alert check
```

### 4. Generate Code

```bash
# Generate Python code
agent dev build "create a hello world function" -l python

# Generate with output file
agent dev build "REST API endpoint" -l go -o api.go
```

### 5. Run Commands

```bash
# PowerShell
agent dev run "Get-Process" -s powershell

# Azure CLI
agent dev run "vm list" -s az

# Auto-detect shell
agent dev run "ls -la"
```

## Configuration

Config file location: `~/.agent/config.yaml`

```yaml
ollama:
  base_url: http://localhost:11434
  model: codellama

anthropic:
  api_key: ""
  model: claude-3-sonnet-20240229

azure:
  auth_method: cli
  subscription_id: ""

aws:
  access_key: ""
  secret_key: ""
  region: us-east-1

gcp:
  project_id: ""

storage:
  path: ~/.agent/data.db
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `AWS_ACCESS_KEY_ID` | AWS access key |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key |
| `AWS_SESSION_TOKEN` | AWS session token |
| `GCP_PROJECT_ID` | GCP project ID |

## Commands

### Cost Management

| Command | Description |
|---------|-------------|
| `agent cost current` | Current month costs |
| `agent cost fetch` | Fetch costs from cloud |
| `agent cost summary` | Cost summary |
| `agent cost history` | Historical trends |
| `agent cost forecast` | Cost prediction |
| `agent cost trend` | Trend analysis |
| `agent cost report` | Generate report |
| `agent cost alert add [name] [threshold]` | Create alert |
| `agent cost alert list` | List alerts |
| `agent cost alert check` | Check alerts |
| `agent cost alert delete [name]` | Delete alert |

### Cloud Providers

| Command | Description |
|---------|-------------|
| `agent cloud list` | List configured providers |
| `agent cloud all` | All providers cost summary |

### Development Tools

| Command | Description |
|---------|-------------|
| `agent dev build [task]` | Generate code |
| `agent dev build [task] -l [language]` | Generate code in specific language |
| `agent dev build [task] -o [file]` | Generate code to file |
| `agent dev review [path]` | Review code |
| `agent dev test [path]` | Run tests |
| `agent dev run [command]` | Execute shell commands |
| `agent dev run [command] -s [shell]` | Execute in specific shell |

### Configuration

| Command | Description |
|---------|-------------|
| `agent config list` | List config |
| `agent config get [key]` | Get value |
| `agent config set [key] [value]` | Set value |

## API Server

Start the REST API server:

```bash
agent-api -port 8080
```

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| GET | `/api/v1/cost/azure/current` | Current costs |
| GET | `/api/v1/cost/azure/summary` | Cost summary |
| GET | `/api/v1/cost/azure/history` | Historical costs |
| GET | `/api/v1/cost/azure/forecast` | Cost forecast |
| GET | `/api/v1/cost/azure/trend` | Trend analysis |
| GET | `/api/v1/cost/all` | All providers |
| GET | `/api/v1/cost/report` | Generate report |
| GET | `/api/v1/alerts` | List alerts |
| POST | `/api/v1/alerts` | Create alert |
| DELETE | `/api/v1/alerts?name=x` | Delete alert |
| GET | `/api/v1/alerts/check` | Check alerts |
| GET | `/api/v1/config` | Get config |

### Example

```bash
# Start server
agent-api -port 8080 &

# Get current costs
curl http://localhost:8080/api/v1/cost/azure/current

# Get config
curl http://localhost:8080/api/v1/config
```

## Output Formats

All commands support multiple output formats:

```bash
# Table (default)
agent cost current

# JSON
agent cost current -o json

# CSV
agent cost current -o csv
```

## Development

### Requirements

- Go 1.21+
- Azure CLI (for Azure auth)
- Ollama (optional, for local AI models)

### Build

```bash
# Build CLI
go build -o agent ./cmd/agent

# Build API server
go build -o agent-api ./cmd/api
```

### Test

```bash
go test ./...
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Support

- Report bugs: [GitHub Issues](https://github.com/yourusername/agent/issues)
