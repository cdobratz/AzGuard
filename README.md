# MissionAgents

## Business Agent System with Cloud Cost Tracking

#### Project Overview

A cross-platform CLI for Windows (PowerShell, Bash, Azure CLI) that builds software AND tracks cloud costs. Powered by Ollama (local) and Anthropic Claude API. Distributed as standalone binaries.

### Capabilities

1. Software Development (from before)

- Build software - Generate code, scaffold projects
- Review code - LLM-powered code review
- Run tests - Execute and analyze test results
- Windows integration - Execute PowerShell, Azure CLI, Batch scripts

2. Cloud Cost Tracking (NEW)

- Current costs - Real-time breakdown by service/resource
- Historical trends - Month-over-month cost comparison
- Budget alerts - Notifications when costs exceed thresholds
- Reporting - Generate cost reports (daily/weekly/monthly)
- Forecasting - Predict future costs based on usage patterns
- Multi-cloud ready - Azure (primary), AWS/GCP in future

---

## All CLI Commands

### Global Flags
```
-o, --output string   Output format: table, json, csv (default "table")
```

### Config Commands
| Command | Description |
|---------|-------------|
| `agent config list` | List all config settings |
| `agent config get [key]` | Get a config value |
| `agent config set [key] [value]` | Set a config value |

### Cost Commands
| Command | Description |
|---------|-------------|
| `agent cost current` | Show current month costs (from Azure) |
| `agent cost fetch` | Fetch and store costs from Azure to local DB |
| `agent cost summary` | Show cost summary from local storage |
| `agent cost history` | Show historical cost trends |
| `agent cost forecast` | Show cost prediction for next month |
| `agent cost trend` | Show month-over-month trend analysis |
| `agent cost alert` | Manage Budget Alerts
| `agent cost report` | Generate report

### Other Commands
| Command | Description |
|---------|-------------|
| `agent completion [shell]` | Generate autocompletion script (bash/zsh/powershell) |

---

## Usage Examples

```bash
# Configuration
agent config list
agent config get azure.subscription_id
agent config set azure.subscription_id <id>
```
```bash
# Cost tracking
agent cost current                 # Current month
agent cost fetch                  # Store costs locally
agent cost summary                # From local DB
agent cost history               # Historical trends
agent cost trend                 # Trend analysis
agent cost forecast              # Next month prediction
```
```bash
# Output formats
agent cost current -o json       # JSON output
agent cost current -o csv        # CSV export
agent cost trend -o json         # JSON for scripting

# Cost data takes ~24-48 hours to appear in Azure after resource usage
```

---

## ✅ Phase 4 Complete

### New Features Added

#### LLM Providers
- **Ollama** - Local models (requires running Ollama server)
- **Anthropic** - Claude API (requires ANTHROPIC_API_KEY)
- Auto-fallback between providers

#### Dev Commands

| Command | Description |
|---------|-------------|
| `agent dev build [task]` | Generate code using AI |
| `agent dev review [path]` | Review code using AI |
| `agent dev test [path]` | Run tests |
| `agent dev run [command]` | Run shell commands |

#### Examples

```bash
# Generate code
agent dev build "create a hello world function in python"
agent dev build "REST API endpoint" -l go -o api.go

# Code review
agent dev review path/to/file.py

# Run tests
agent dev test path/to/test.py

# Run commands
agent dev run "Get-Process" -s powershell      # PowerShell
agent dev run "ls -la" -s bash                  # Bash
agent dev run "vm list" -s az                   # Azure CLI
agent dev run "dir" -s cmd                      # CMD
```

---

### All CLI Commands

| Category | Command | Description |
|----------|---------|-------------|
| **Config** | `agent config` | Manage configuration |
| **Cost** | `agent cost` | Cloud cost tracking |
| **Dev** | `agent dev build` | Code generation |
| | `agent dev review` | Code review |
| | `agent dev test` | Run tests |
| | `agent dev run` | Shell execution |

