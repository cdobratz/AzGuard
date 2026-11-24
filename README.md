# azguard

**Find and clean up forgotten and AI-agent-created cloud resources on Azure and AWS.**

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)

azguard is a local, open source CLI. There is no hosted service and no paid tier.

## Install

Windows (Scoop). The bucket is [cdobratz/scoop-azguard](https://github.com/cdobratz/scoop-azguard). The app name is `azguard`. A tagged release updates the manifest. See [UPDATE.md](UPDATE.md).

```powershell
scoop bucket add azguard https://github.com/cdobratz/scoop-azguard
scoop install azguard
```

macOS and Linux (Homebrew). The tap is [cdobratz/homebrew-azguard](https://github.com/cdobratz/homebrew-azguard). The formula is `azguard`. A tagged release updates the formula. See [UPDATE.md](UPDATE.md).

```bash
brew install cdobratz/azguard/azguard
```

Or use the website installer (macOS and Linux). That script is hosted at [azguard.dev](https://azguard.dev). It is not stored in this repository. It downloads the latest GitHub release and checks `checksums.txt` before it installs.

```bash
curl -sSL https://azguard.dev/install.sh | bash
```

You can also download the archive for your OS and architecture from the [latest release](https://github.com/cdobratz/AzGuard/releases/latest) (`azguard_<version>_<os>_<arch>.tar.gz` or `.zip`, plus `checksums.txt`). Verify the archive against `checksums.txt`, extract it, and put the `azguard` binary on your `PATH`.

Build from source (Go 1.26 or later):

```bash
git clone https://github.com/cdobratz/AzGuard.git
cd AzGuard
go build -o azguard ./cmd/azguard
```

Or install from the module:

```bash
go install github.com/cdobratz/azguard/cmd/azguard@latest
```

## Quick start

Sign in with the Azure CLI (the default auth method). If `subscription_id` is empty, azguard can read it from `az account show`.

```bash
az login
```

Edit `~/.azguard/config.yaml` to set a subscription or AWS region yourself. Then record the account type once:

```bash
azguard init --azure payg --aws paid
azguard init --azure free --aws free --azure-credit 50 --azure-credit-expires 2026-12-31
```

```bash
# Dry run: show what would be deleted. Nothing is deleted.
azguard cleanup

# Delete after you type yes. --yes skips the prompt.
azguard cleanup --apply

# Read-only list of idle and orphaned resources
azguard resources

# Spend, account type, and any credit stored by init
azguard status

# Compare recorded usage with free-tier limits
azguard scan
```

`azguard cleanup` is a dry run unless you pass `--apply`. A resource tagged `azguard:keep=true` (also `yes` or `1`) is never deleted. If tags could not be loaded, the resource is skipped. Only findings marked `safe_to_delete` are eligible unless you pass `--include-unsafe`.

## Commands

| Command | Description |
|---------|-------------|
| `azguard cleanup` | Delete idle and orphaned resources (dry run unless `--apply`; `azguard:keep=true` is kept) |
| `azguard resources` | Find idle and orphaned resources, with a monthly cost estimate |
| `azguard watch` | Poll for new billable resources and alert (default interval 15 minutes; not a live stream) |
| `azguard guard` | Block billable commands that exceed a session budget |
| `azguard mcp serve` | Stdio MCP server for prevention checks |
| `azguard preflight` | Approximate monthly price of a Terraform plan or a cloud command |
| `azguard status` | Spend, account type, and any credit stored by `azguard init` |
| `azguard scan` | Compare recorded usage with Azure free-tier limits |
| `azguard init` | Record Azure (`free` or `payg`) and AWS (`free` or `paid`) account type |
| `azguard budget add [amount]` | Add a budget alert ($1-$100) |
| `azguard budget list` | List all budget alerts |
| `azguard cost current` | Show current month costs |
| `azguard cost history` | Show cost history |
| `azguard cost fetch` | Fetch and store costs from Azure |
| `azguard aws status` | Check your AWS free tier usage |
| `azguard aws scan` | Scan AWS services approaching limits |
| `azguard aws alerts --threshold 80` | Set alert thresholds (percentage) |
| `azguard aws resources` | List free tier eligible services |
| `azguard aws cost` | View AWS cost breakdown |

```bash
azguard resources --output json
azguard resources --cloud aws --region us-east-1
azguard resources --all-regions
azguard cleanup --cloud aws --region us-east-1 --type ebs --older-than 30d
azguard cleanup --apply --yes --type disk --tag env=dev
azguard watch --once
azguard watch --interval 15m --webhook "$AZGUARD_WEBHOOK"
azguard guard --budget 25 -- aws ec2 run-instances --instance-type t3.micro
azguard guard hook --budget 25
azguard preflight --plan plan.json
azguard preflight --command "az vm create -g rg -n vm --size Standard_B1s"
```

`--output` (or `-o`) is `table`, `json`, or `csv` on the root command. `azguard resources --fail-over 25` exits 2 when the estimated monthly total is greater than $25.

See `azguard --help` for `budget remove`, `budget presets`, `cost forecast`, `config list`, `guard report`, and `version`.

The composite action in `action.yml` is not on the GitHub Marketplace. Call it from a checkout of this repository, or from a ref you pin. An example workflow is `examples/azguard.yml`. It runs `azguard preflight` on a `terraform show -json` plan and can comment on a pull request.

## Configuration

Config file: `~/.azguard/config.yaml`

```yaml
azure:
  auth_method: cli  # or service_principal
  subscription_id: YOUR_SUB_ID
  # Do not put client_secret in this file.

aws:
  region: us-east-1  # optional, uses AWS CLI default
  # Do not put access keys in this file.

storage:
  path: ~/.azguard/data.db

guard:
  budget_usd: 0  # USD per month for azguard guard. --budget overrides this. 0 warns and allows.
```

Cloud secrets are read from the environment (`AZURE_CLIENT_SECRET`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`) or from the OS keychain (service name `azguard`). If a config file still has those fields, the next start copies them into the keychain and removes them from the file. When the keychain is unavailable, the file is left as it is.

`azguard init` records the account type once. `status` uses that answer when the cloud API does not say. A Pay-As-You-Go or Paid account is not shown as a percentage of a $200 free tier.

Azure uses `az login`, a service principal, or managed identity. AWS uses the OS keychain, the environment, or the AWS CLI credential chain (`aws configure export-credentials`, which includes profiles and SSO).

## License

MIT License - see [LICENSE](LICENSE).

## Support

- Email: [support@azguard.dev](mailto:support@azguard.dev)
- Website: [azguard.dev](https://azguard.dev)
- [GitHub Issues](https://github.com/cdobratz/AzGuard/issues)

See [CONTRIBUTING.md](CONTRIBUTING.md) if you want to contribute.
