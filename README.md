# azguard

**One command to make sure your Azure and AWS free tiers don't surprise you with a bill.**

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8.svg)

## The Problem

Azure's free tier has 65+ services with different expiration rules, usage caps, and hidden dependencies. A "free" VM triggers billable disks, IPs, and monitoring logs. Thousands of students, freelancers, and small teams get surprised by unexpected charges every month.

## The Solution

azguard monitors your Azure and AWS usage against free tier limits and alerts you before you accidentally accumulate charges.

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

### Azure

| Command | Description |
|---------|-------------|
| `azguard status` | Quick overview of your Azure free tier status |
| `azguard scan` | Scan for Azure free tier overages |
| `azguard resources` | Find idle and orphaned resources, with a monthly cost estimate |
| `azguard budget add [amount]` | Add a budget alert ($1-$100) |
| `azguard budget list` | List all budget alerts |
| `azguard cost current` | Show current month costs |
| `azguard cost history` | Show cost history |
| `azguard cleanup` | Interactive cleanup guide |

### AWS

| Command | Description |
|---------|-------------|
| `azguard aws status` | Check your AWS free tier usage |
| `azguard aws scan` | Scan AWS services approaching limits |
| `azguard aws alerts --threshold 80` | Set alert thresholds (percentage) |
| `azguard aws resources` | List free tier eligible services |
| `azguard aws cost` | View AWS cost breakdown |

## Installation

### One-Liner (Recommended)

```bash
curl -sSL https://azguard.dev/install.sh | bash
```

### Package Managers

```powershell
# Scoop (Windows)
scoop bucket add azguard https://github.com/cdobratz/scoop-azguard
scoop install azguard
```

```bash
# Homebrew (macOS/Linux)
brew install cdobratz/azguard/azguard
```

```powershell
# Chocolatey (Windows)
choco install azguard
```

### Build from Source

```bash
git clone https://github.com/cdobratz/AzGuard.git
cd AzGuard
go build -o azguard ./cmd/agent
```

## Configuration

Config file: `~/.azguard/config.yaml`

```yaml
azure:
  auth_method: cli  # or service_principal
  subscription_id: YOUR_SUB_ID

aws:
  region: us-east-1  # optional, uses AWS CLI default
  # credentials resolved from: config > env vars > AWS CLI

storage:
  path: ~/.azguard/data.db
```

## Idle and orphaned resources

`azguard resources` is read-only. It finds resources that are easy to forget and that keep billing: unattached disks, idle public IPs, NAT gateways, stopped-but-still-billed VMs, old snapshots, and the AWS equivalents. It does not delete anything.

```bash
azguard resources
azguard resources --output json
azguard resources --output csv --fail-over 25
azguard resources --cloud aws --region us-east-1
azguard resources --all-regions
azguard resources --rules
azguard resources --schema
```

AWS is scanned in the configured region unless you pass `--all-regions`. `--snapshot-days` defaults to 30. `--fail-over 25` exits **2** when the deduplicated estimated monthly total is greater than $25. A negative value (the default) disables that check.

`safe_to_delete` means nothing is attached to the resource. It does not mean deletion is free of data loss. Read `safe_to_delete_hint`. Created-by is filled from Azure `systemData.createdBy` or an AWS `CreatedBy` / `Owner` tag when that value exists.

Estimates use the Azure Retail Prices API and the AWS Price List API when those calls succeed, and a bundled list-price table otherwise (730 hours per month). `price_source` on each finding is `azure-retail`, `aws-price-list`, `bundled`, or `unpriced`. The figure is a list price, not an invoice. Two rules can match one resource (an idle Elastic IP is both `aws.eip.unassociated` and `aws.ipv4.public`); `total_estimated_monthly_usd` counts that resource once.

### JSON schema (`azguard.findings.v1`)

`azguard resources --output json` prints an object with `schema`, `schema_version`, `generated_at`, `findings`, `total_estimated_monthly_usd`, `currency`, and `errors`. Each finding has `resource_id`, `name`, `provider`, `region`, `rule_id`, `title`, `evidence`, `estimated_monthly_usd`, `currency`, `created_by`, `safe_to_delete`, `safe_to_delete_hint`, and `price_source`. `azguard resources --schema` prints the JSON Schema document.

CSV columns are the finding fields above, in that order.

### Rules

| Rule | What it flags |
|------|----------------|
| `azure.disk.unattached` | Managed disk with `diskState` Unattached |
| `azure.publicip.unassociated` | Public IP with no IP configuration |
| `azure.nic.unattached` | NIC with no VM and no private endpoint |
| `azure.vm.stopped_allocated` | VM power state Stopped (allocated), which is still billed. Deallocated VMs are not listed |
| `azure.appserviceplan.empty` | App Service plan with zero sites |
| `azure.snapshot.old` | Snapshot older than `--snapshot-days` |
| `azure.natgateway.no_subnets` | NAT gateway with no subnets |
| `azure.loganalytics.empty_ingestion` | Workspace with no solutions that is still ingesting data |
| `aws.ebs.unattached` | EBS volume in `available` with no attachments |
| `aws.eip.unassociated` | Elastic IP that is not associated |
| `aws.ipv4.public` | Every public IPv4 address (charged whether or not it is attached) |
| `aws.natgateway.idle` | NAT gateway with 0 `BytesOutToDestination` over 7 days |
| `aws.vpcendpoint.interface` | Interface VPC endpoint (gateway endpoints are free and omitted) |
| `aws.ec2.stopped_with_ebs` | Stopped instance that still has EBS attached |
| `aws.elb.no_healthy_targets` | Load balancer with no healthy targets |
| `aws.snapshot.old` | Snapshot older than `--snapshot-days` |

### Read-only permissions

Give the Azure subscription the built-in **Reader** role. The calls are:

- `Microsoft.ResourceGraph/resources/read`
- `Microsoft.Resources/subscriptions/resources/read`
- `Microsoft.OperationalInsights/workspaces/usages/read`

The Retail Prices API at `prices.azure.com` is public and does not use subscription credentials.

On AWS, attach **ReadOnlyAccess**, or only these actions:

- `ec2:DescribeVolumes`
- `ec2:DescribeAddresses`
- `ec2:DescribeNetworkInterfaces`
- `ec2:DescribeNatGateways`
- `ec2:DescribeVpcEndpoints`
- `ec2:DescribeInstances`
- `ec2:DescribeSnapshots`
- `ec2:DescribeRegions`
- `elasticloadbalancing:DescribeLoadBalancers`
- `elasticloadbalancing:DescribeTargetGroups`
- `elasticloadbalancing:DescribeTargetHealth`
- `cloudwatch:GetMetricStatistics`
- `pricing:GetProducts`

Do not grant delete, terminate, release, stop, or deallocate permissions for this command.

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
- [x] AWS free tier guard
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
