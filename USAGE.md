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

The bucket name is `azguard`. The manifest lives in `cdobratz/scoop-azguard` (`bucket/azguard.json`), not in this repository.

```powershell
scoop bucket add azguard https://github.com/cdobratz/scoop-azguard
scoop install azguard
```

### Homebrew (macOS and Linux)

The tap is `cdobratz/azguard` and the formula is `azguard` (`Formula/azguard.rb` in `cdobratz/homebrew-azguard`).

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
azguard guard hook --budget 25
azguard guard report --fixture activity.json

# Stdio MCP server for prevention checks (idle resources, preflight, approximate prices)
azguard mcp serve

# Approximate monthly price of a plan or a command (JSON on stdout)
azguard preflight --plan plan.json
azguard preflight --command "aws ec2 run-instances --instance-type t3.micro"

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

### Agent guard hook

`azguard guard -- <command>` runs the command only when the session budget allows it. `azguard guard hook` reads a Claude Code PreToolUse payload on stdin and prints `permissionDecision` `allow` or `deny`. Read-only commands pass through. Billable commands (`az ... create`, `aws ... run-instances`, `aws ... create-*`, `terraform apply`, `pulumi up`, `cdk deploy`) show an estimated monthly cost and are blocked when the session budget is exceeded.

The budget is `--budget`, or `guard.budget_usd` in `~/.azguard/config.yaml` when the flag is omitted and the config value is greater than zero.

`azguard guard report --fixture activity.json` lists creates from a recorded Activity Log or CloudTrail page. It does not call the cloud.

Sample `.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{ "type": "command", "command": "azguard guard hook --budget 25" }]
      }
    ]
  }
}
```

### MCP server

`azguard mcp serve` speaks the Model Context Protocol on stdin and stdout. The tools are for prevention. They do not look up current spend.

Prices are the bundled list-price table (730 hours per month). They are approximate and are not an invoice.

| Tool | Input | Output |
|------|-------|--------|
| `list_idle_resources` | `cloud` (optional): `all`, `azure`, or `aws` | Idle and orphaned findings, same rules as `azguard resources` |
| `list_new_billable_resources` | `since` (required, RFC3339), `principal` (optional) | Billable creates from the Activity Log and CloudTrail |
| `check_free_or_credit_status` | `cloud` (optional) | Bundled free-tier limits and credit stored by `azguard init` |
| `estimate_monthly_cost` | `provider` and `kind` (required). `sku`, `quantity`, and `region` optional | Approximate monthly price for one resource |
| `preflight` | `input`, or `command` plus `plan` | A sentence such as "this will create a NAT Gateway at about $32.85/month" |

`preflight` input may be `terraform show -json` output, or a command such as `az vm create -g rg -n vm --size Standard_B1s` or `aws ec2 run-instances --instance-type t3.micro`. A read-only command is reported as not billable.

`kind` for `estimate_monthly_cost` is one of `vm`, `nat_gateway`, `public_ip`, `disk`, `ebs`, `instance`, `load_balancer`, `vpc_endpoint`, `snapshot`, or `app_service_plan`. `quantity` is a count, or a size in GB for disks, volumes, and snapshots.

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
