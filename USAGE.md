# azguard Usage Guide

## Table of Contents

1. [Installation](#installation)
2. [Configuration](#configuration)
3. [Commands](#commands)
4. [Examples](#examples)

---

## Installation

### One-Liner (Recommended)

```bash
curl -sSL https://azguard.dev/install.sh | bash
```

### Manual Install

```bash
# Download latest release
curl -L -o azguard https://github.com/azguard/azguard/releases/latest/download/azguard

# Make executable
chmod +x azguard

# Add to PATH
sudo mv azguard /usr/local/bin/
```

### Package Managers

```powershell
# Scoop (Windows)
scoop bucket add extras
scoop install azguard

# Homebrew (macOS/Linux)
brew install azguard

# Chocolatey (Windows)
choco install azguard
```

---

## Configuration

### Initial Setup

```bash
# Create config directory
mkdir -p ~/.azguard

# Set your Azure subscription
azguard config set subscription YOUR_SUBSCRIPTION_ID
```

### Authentication

azguard uses your existing Azure CLI credentials:

```bash
# Login to Azure
az login
```

That's it! azguard will use your Azure credentials automatically.

### Config File

Location: `~/.azguard/config.yaml`

```yaml
azure:
  auth_method: cli
  subscription_id: YOUR_SUB_ID
  tenant_id:        # Optional (for service principal)
  client_id:        # Optional (for service principal)
  client_secret:    # Optional (for service principal)

storage:
  path: ~/.azguard/data.db
```

---

## Commands

### Quick Status

```bash
azguard status
```

Shows:
- Current spend vs free tier limit
- Active budget alerts
- Status (OK / Warning / Over)

### Scan for Overages

```bash
azguard scan
```

Audits your subscription against Azure free tier limits and shows:
- Total spend vs free tier
- Per-service breakdown
- Warning/overage indicators

### Budget Alerts

```bash
# Add a budget alert
azguard budget add 5      # $5 budget
azguard budget add 10     # $10 budget

# List all alerts
azguard budget list

# Show preset options
azguard budget presets

# Remove an alert
azguard budget remove budget-5
```

### Cost Commands

```bash
# Fetch latest costs from Azure
azguard cost fetch

# Current month costs
azguard cost current

# Historical costs
azguard cost history
azguard cost history --days 90

# Cost forecast
azguard cost forecast
```

### Resources

```bash
# Check running resources
azguard resources

# Cleanup guide
azguard cleanup
```

### Configuration

```bash
# List config
azguard config list

# Set config
azguard config set subscription YOUR_SUB_ID
```

---

## Examples

### Daily Check

```bash
#!/bin/bash
# Daily Azure bill check

echo "=== Azure Free Tier Status ==="
azguard status
```

### Cron Job for Monitoring

```bash
# Run every day at 8am
0 8 * * * /usr/local/bin/azguard status
```

### CI/CD Integration

```bash
# In your CI pipeline
azguard status
if [ $? -eq 0 ]; then
  echo "All good!"
else
  echo "Warning: Check azguard status"
fi
```

---

## Output Formats

All commands support multiple output formats:

```bash
# Table (default)
azguard status

# JSON (for scripting)
azguard status -o json

# JSON with jq
azguard status -o json | jq '.total_spend'
```

---

## Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| No subscription found | Run `az login` first |
| Costs show $0.00 | Wait 24-48 hours for billing data |
| Permission denied | Ensure you have Cost Management Reader role |

### Get Help

```bash
# Show help
azguard --help

# Show help for specific command
azguard scan --help
```
