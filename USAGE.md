# azguard Usage Guide

azguard finds forgotten and AI-agent-created resources on Azure and AWS, then deletes only what you confirm. `azguard cleanup` is a dry run unless you pass `--apply`. A resource tagged `azguard:keep=true` is never deleted.

Free-tier and credit tracking is the second part. `azguard status` and `azguard scan` show spend and account type. Credit remaining and expiry are the values stored by `azguard init` (`--azure-credit`, `--azure-credit-expires`, `--aws-credit`, `--aws-credit-expires`). A Pay-As-You-Go or Paid account is not shown as a percentage of a $200 free tier.

The CLI is local. It does not host your account and it has no paid tier.

## Table of Contents

1. [Installation](#installation)
2. [Configuration](#configuration)
3. [Commands](#commands)
4. [Examples](#examples)

---

## Installation

### Scoop (Windows)

The bucket name is `azguard`. The manifest lives in `cdobratz/scoop-azguard` (`bucket/azguard.json`), not in this repository. A tagged release updates that manifest. See [UPDATE.md](UPDATE.md).

```powershell
scoop bucket add azguard https://github.com/cdobratz/scoop-azguard
scoop install azguard
```

### Homebrew (macOS and Linux)

The tap is `cdobratz/azguard` and the formula is `azguard` (`Formula/azguard.rb` in `cdobratz/homebrew-azguard`). A tagged release publishes a cask at `Casks/azguard.rb` and does not remove that formula. Until the formula is removed, keep the command below. See [UPDATE.md](UPDATE.md).

```bash
brew install cdobratz/azguard/azguard
```

### Website installer (macOS and Linux)

```bash
curl -sSL https://azguard.dev/install.sh | bash
```

That script is hosted at azguard.dev. It is not stored in this repository. It downloads the latest GitHub release and checks `checksums.txt` before it installs. On Windows, use Scoop.

### Manual Install

Download the archive for your OS and architecture from the [latest release](https://github.com/cdobratz/AzGuard/releases/latest) (`azguard_<version>_<os>_<arch>.tar.gz` or `.zip`, plus `checksums.txt`). Verify the archive against `checksums.txt`, extract it, and put the `azguard` binary on your `PATH`.

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
  # client_secret is not stored here. Use AZURE_CLIENT_SECRET or the OS keychain.

storage:
  path: ~/.azguard/data.db
```

`azguard init --azure free|payg --aws free|paid` stores the account type once. `status` uses that answer when the cloud API does not. Pay-As-You-Go and Paid accounts are not shown as a percent of a $200 free tier.

---

## Commands

### Quick Status

```bash
azguard status
```

Shows:
- Azure account type (free with a spending limit, or Pay-As-You-Go)
- Current spend
- Credit remaining and expiry when `azguard init` stored them
- Active budget alerts

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
# Find idle and orphaned resources (read-only; exit 2 above --fail-over)
azguard resources
azguard resources --output json --fail-over 25

# Poll Activity Log and CloudTrail (default every 15m; not a live stream)
azguard watch --once
azguard watch --webhook "$AZGUARD_WEBHOOK"

# Block a billable command when the session budget is exceeded (exit 2)
azguard guard --budget 25 -- aws ec2 run-instances --instance-type t3.micro
azguard guard report --fixture activity.json

# Dry run: list what cleanup would delete (id, type, region, age, cost)
azguard cleanup
azguard cleanup --type disk --region eastus --older-than 30d

# Delete after you type yes. --yes skips the prompt. azguard:keep=true is never deleted.
azguard cleanup --apply
azguard cleanup --apply --yes --cloud aws --type ebs
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
