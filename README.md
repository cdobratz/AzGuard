# MissionAgents

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
