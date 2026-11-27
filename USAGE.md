# azguard usage

This guide covers commands and flags. For install, support, and a short quick start, see [README.md](README.md).

azguard is a local, open source CLI. There is no hosted service and no paid tier.

`azguard cleanup` is a dry run unless you pass `--apply`. A resource tagged `azguard:keep=true` (also `yes` or `1`) is never deleted. If tags could not be loaded, the resource is skipped. Only findings marked `safe_to_delete` are eligible unless you pass `--include-unsafe`.

## Configure

Sign in with the Azure CLI (the default auth method). If `subscription_id` is empty, azguard can read it from `az account show`.

```bash
az login
azguard config list
```

`azguard config list` prints the subscription, auth method, and storage path that the CLI loaded. Edit `~/.azguard/config.yaml` to set a subscription or AWS region yourself. Do not use `azguard config set` to configure the CLI. That command writes to the SQLite database, and the process loads settings from the yaml file and from `az account show`.

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

Record the account type once:

```bash
azguard init --azure payg --aws paid
azguard init --azure free --aws free --azure-credit 50 --azure-credit-expires 2026-12-31
```

`azguard init` also accepts `--aws-spend-limit yes|no`, `--aws-credit`, `--aws-credit-expires`, and `--force` to replace stored answers. `status` uses the stored account type when the cloud API does not say. A Pay-As-You-Go or Paid account is not shown as a percentage of a $200 free tier.

Cloud secrets are read from the environment (`AZURE_CLIENT_SECRET`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`) or from the OS keychain (service name `azguard`). Azure uses `az login`, a service principal, or managed identity. AWS uses the OS keychain, the environment, or the AWS CLI credential chain (`aws configure export-credentials`, which includes profiles and SSO).

## Cleanup and discovery

```bash
azguard resources
azguard resources --output json --fail-over 25
azguard resources --cloud aws --region us-east-1
azguard resources --all-regions
azguard resources --rules
azguard resources --schema

azguard cleanup
azguard cleanup --type disk --region eastus --older-than 30d
azguard cleanup --apply
azguard cleanup --apply --yes --cloud aws --type ebs --tag env=dev

azguard watch
azguard watch --once
azguard watch --interval 15m --webhook "$AZGUARD_WEBHOOK"
```

`--fail-over 25` exits 2 when the estimated monthly total is greater than $25. `--older-than` takes a duration such as `30d` or `720h`. Repeat `--type`, `--tag key=value`, and `--region`. `--cloud` is `all` (the default), `azure`, or `aws`.

`azguard watch` polls for new billable creates. The default interval is 15 minutes. It is not a live stream.

## Account and cost

```bash
azguard status
azguard scan

azguard budget add 5
azguard budget list
azguard budget presets
azguard budget remove budget-5

azguard cost fetch
azguard cost current
azguard cost current --output json
azguard cost history
azguard cost forecast

azguard aws status
azguard aws scan
azguard aws alerts --threshold 80
azguard aws resources
azguard aws cost
```

`azguard budget add` accepts an amount from 1 to 100 and stores the alert as `budget-<amount>`, for example `budget-5`. `azguard cost history` has no `--days` flag. It shows cost for the last 30 days.

The root `--output` (or `-o`) flag is `table`, `json`, or `csv`. `azguard resources`, `azguard cleanup`, and `azguard cost current` / `azguard cost history` honor it. `azguard status` and `azguard scan` print text even if you pass `-o json`. For cost JSON, the total field is `total_cost`.

## Guard

`azguard guard -- <command>` runs the command only when the session budget allows it. The wrapper exits 2 without running the command when it blocks. A budget of 0 warns and allows. Read-only commands pass through.

The budget is `--budget` when you pass it. Otherwise `guard.budget_usd` in `~/.azguard/config.yaml` is used when that value is greater than zero.

```bash
azguard guard --budget 25 -- aws ec2 run-instances --instance-type t3.micro
azguard guard --budget 25 --plan plan.json -- terraform apply tfplan
azguard guard hook --budget 25
azguard guard report --since 2026-10-17T12:00:00Z --principal casey@contoso.com
azguard guard report --fixture activity.json
```

`azguard guard hook` reads a Claude Code PreToolUse payload on stdin and writes an allow or deny decision on stdout. It exits 0 so Claude Code can read the JSON. `azguard guard` with no command does the same thing. Billable commands include `az ... create`, `aws ... run-instances`, `aws ... create-*`, `terraform apply`, `pulumi up`, and `cdk deploy`. `terraform apply`, `pulumi up`, and `cdk deploy` stay unpriced unless you pass `--plan`.

`azguard guard report` lists billable creates from the Activity Log and CloudTrail since the session started, or since `--since` (RFC3339). `--fixture` reads a recorded page instead of calling the cloud.

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

## MCP server

`azguard mcp serve` speaks the Model Context Protocol on stdin and stdout. The tools are for prevention. They do not look up current spend, and they do not delete anything. Prices come from the bundled list-price table (730 hours per month) and are not an invoice.

```bash
azguard mcp serve
```

| Tool | Input | Output |
|------|-------|--------|
| `list_idle_resources` | `cloud` (optional): `all`, `azure`, or `aws` | Idle and orphaned findings, same rules as `azguard resources` |
| `list_new_billable_resources` | `since` (required, RFC3339), `principal` (optional) | Billable creates from the Activity Log and CloudTrail |
| `check_free_or_credit_status` | `cloud` (optional) | Bundled free-tier limits and credit stored by `azguard init` |
| `estimate_monthly_cost` | `provider` and `kind` (required). `sku`, `quantity`, and `region` optional | Approximate monthly price for one resource |
| `preflight` | `input`, or `command` plus `plan` | A sentence such as "this will create a NAT Gateway at about $32.85/month" |

`kind` for `estimate_monthly_cost` includes `vm`, `nat_gateway`, `public_ip`, `disk`, `ebs`, `instance`, `load_balancer`, `vpc_endpoint`, `snapshot`, and `app_service_plan`. `quantity` is a count, or a size in GB for disks, volumes, and snapshots.

## Preflight and GitHub Action

```bash
azguard preflight --plan plan.json
azguard preflight --command "az vm create -g rg -n vm --size Standard_B1s"
azguard preflight --command "aws ec2 run-instances --instance-type t3.micro"
```

`azguard preflight` prints JSON on stdout. The figure is the bundled list price, not an invoice.

The composite action in `action.yml` is not on the GitHub Marketplace. Call it from a checkout of this repository, or from a ref you pin. An example workflow is `examples/azguard.yml`. It downloads a release, runs `azguard preflight` on a `terraform show -json` plan, and can comment on a pull request. It fails the check when the approximate monthly total is greater than the `budget` input. Set `scan-idle` to `true` to add idle-resource findings when cloud credentials are already configured. A missing login is noted and does not fail the job by itself.

## Help

```bash
azguard --help
azguard cleanup --help
```
