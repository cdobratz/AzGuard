# azguard

**One command to make sure your Azure free tier doesn't surprise you with a bill.**

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8.svg)

## The Problem

Azure's free tier has 65+ services with different expiration rules, usage caps, and hidden dependencies. A "free" VM triggers billable disks, IPs, and monitoring logs. Thousands of students, freelancers, and small teams get surprised by unexpected charges every month.

## The Solution

azguard monitors your Azure usage against free tier limits and alerts you before you accidentally accumulate charges.

## Features

- **Free Tier Scanner** - Scan your subscription for potential overages
- **Budget Alerts** - Set custom alerts ($1-$100) to prevent bill shock
- **Cost Tracking** - Monitor current spend against free tier limits
- **Resource Cleanup** - Interactive guide to identify unused resources
- **Historical Trends** - Track spending over time

## Quick Start

```bash
# One-line install (recommended)
curl -sSL https://azguard.dev/install.sh | bash

# Or download from releases
curl -L -o azguard https://github.com/azguard/azguard/releases/latest/download/azguard
chmod +x azguard
```

### Configure

```bash
# Set your Azure subscription
azguard config set subscription YOUR_SUBSCRIPTION_ID

# Or use Azure CLI auth (default)
az login
```

### Basic Usage

```bash
# Quick status check
azguard status

# Scan for free tier overages
azguard scan

# Add a budget alert
azguard budget add 5

# Fetch latest costs
azguard cost fetch
```

## Commands

| Command | Description |
|---------|-------------|
| `azguard status` | Quick overview of your free tier status |
| `azguard scan` | Scan for free tier overages |
| `azguard resources` | List resources with status indicators |
| `azguard budget add [amount]` | Add a budget alert ($1-$100) |
| `azguard budget list` | List all budget alerts |
| `azguard cost current` | Show current month costs |
| `azguard cost history` | Show cost history |
| `azguard cleanup` | Interactive cleanup guide |

## Installation

### One-Liner (Recommended)

```bash
curl -sSL https://azguard.dev/install.sh | bash
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

### Build from Source

```bash
git clone https://github.com/azguard/azguard.git
cd azguard
go build -o azguard ./cmd/agent
```

## Configuration

Config file: `~/.azguard/config.yaml`

```yaml
azure:
  auth_method: cli  # or service_principal
  subscription_id: YOUR_SUB_ID

storage:
  path: ~/.azguard/data.db
```

## How It Works

1. **Authentication** - Uses your existing Azure CLI credentials (`az login`)
2. **Cost Query** - Queries Azure Cost Management API (always free)
3. **Limit Check** - Compares usage against known free tier limits
4. **Alert** - Notifies you when approaching or exceeding limits

## Budget Presets

```bash
$1   Strict budget - great for free tier testing
$5   Small budget - light usage
$10  Medium budget - moderate usage
$20  Higher budget - warning before limit
```

## Use Cases

- **Students** - Learning Azure without accumulating charges
- **Freelancers** - Client projects on limited budgets
- **Bootcamp Grads** - First cloud experience
- **Small Teams** - Quick cost visibility without enterprise tools

## Why azguard?

| Tool | Focus | Price |
|------|-------|-------|
| Azure Portal | Enterprise billing | Free but complex |
| Infracost | Terraform costs | Free + Paid |
| CloudZero | Enterprise FinOps | $30K+/year |
| **azguard** | Free tier protection | **Free, open source** |

## Roadmap

- [ ] Daily/weekly monitoring with notifications
- [ ] AWS free tier guard
- [ ] GCP free tier guard
- [ ] Slack/Teams notifications
- [ ] Web dashboard

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT License - see [LICENSE](LICENSE).

## Support

- [GitHub Issues](https://github.com/azguard/azguard/issues)
- [GitHub Discussions](https://github.com/azguard/azguard/discussions)
