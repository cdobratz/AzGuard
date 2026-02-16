# Usage Guide

## Table of Contents

1. [Configuration](#configuration)
2. [Cost Management](#cost-management)
3. [Cloud Providers](#cloud-providers)
4. [Development Tools](#development-tools)
5. [API Server](#api-server)

---

## Configuration

### Initial Setup

1. Copy the default config:
   ```bash
   mkdir -p ~/.agent
   cp configs/config.yaml ~/.agent/config.yaml
   ```

2. Edit `~/.agent/config.yaml` with your settings

### Azure Configuration

```yaml
azure:
  auth_method: cli              # cli, service_principal, managed_identity
  subscription_id: YOUR_SUB_ID  # Your Azure subscription ID
  tenant_id: YOUR_TENANT_ID     # For service principal
  client_id: YOUR_CLIENT_ID    # For service principal
  client_secret: YOUR_SECRET   # For service principal
```

**Authentication Methods:**

| Method | Description |
|--------|-------------|
| `cli` | Use `az login` (default, easiest) |
| `service_principal` | Use app registration |
| `managed_identity` | Use Azure VM/Container identity |

### AWS Configuration

```yaml
aws:
  access_key: YOUR_ACCESS_KEY
  secret_key: YOUR_SECRET_KEY
  session_token: YOUR_SESSION_TOKEN  # Optional, for temp credentials
  region: us-east-1
```

Or via environment variables:
```bash
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
```

### GCP Configuration

```yaml
gcp:
  project_id: YOUR_PROJECT_ID
```

Or via environment variables:
```bash
export GCP_PROJECT_ID=...
```

### LLM Providers

```yaml
ollama:
  base_url: http://localhost:11434
  model: codellama

anthropic:
  api_key: YOUR_API_KEY
  model: claude-3-sonnet-20240229
```

### Config Commands

```bash
# List all config
agent config list

# Get specific value
agent config get azure.subscription_id
agent config get aws.region

# Set value
agent config set azure.subscription_id 90f4b6d4-2401-43c1-9c92-14abdfdb2e01
agent config set aws.region us-east-1
```

---

## Cost Management

### Fetching Costs

```bash
# Fetch current month costs from Azure
agent cost current

# Fetch and store costs locally
agent cost fetch
```

### Viewing Costs

```bash
# Current month summary
agent cost current

# From local database
agent cost summary

# Historical data (default: last 30 days)
agent cost history
agent cost history --days 90
```

### Analysis

```bash
# Trend analysis (month-over-month)
agent cost trend

# Cost forecast
agent cost forecast
```

### Reports

```bash
# Generate report (default: last 12 months)
agent cost report

# Output formats
agent cost report -o json
agent cost report -o csv
```

### Budget Alerts

```bash
# Create alert
agent cost alert add monthly-budget 100

# List alerts
agent cost alert list

# Check alerts against current costs
agent cost alert check

# Delete alert
agent cost alert delete monthly-budget
```

### Output Formats

```bash
# Table (default)
agent cost current

# JSON (for scripting)
agent cost current -o json

# CSV (for spreadsheets)
agent cost current -o csv
```

---

## Cloud Providers

### List Configured Providers

```bash
agent cloud list
```

Output:
```
☁️  Configured Cloud Providers
─────────────────────────────
✅ Azure: 90f4b6d4-2401-43c1-9c92-14abdfdb2e01
✅ AWS: us-east-1
❌ GCP: Not configured
```

### All Providers Summary

```bash
agent cloud all
```

---

## Development Tools

### Code Generation

```bash
# Generate code (default: Python)
agent dev build "create a hello world function"

# Specify language
agent dev build "REST API endpoint" -l go
agent dev build "hello world" -l javascript
agent dev build "data class" -l java

# Save to file
agent dev build "create user model" -l python -o models.py

# Available languages: python, go, javascript, typescript, java, rust, csharp
```

### Code Review

```bash
# Review a file
agent dev review path/to/file.py

# With JSON output
agent dev review path/to/file.py -o json
```

### Test Execution

```bash
# Run tests
agent dev test path/to/test.py

# Works with:
# - Python: pytest
# - JavaScript: npm test / jest
# - Go: go test
# - Rust: cargo test
# - Java: mvn test
```

### Shell Execution

```bash
# PowerShell
agent dev run "Get-Process | Select-Object -First 5" -s powershell

# Bash
agent dev run "ls -la" -s bash

# Azure CLI
agent dev run "vm list --output table" -s az

# CMD
agent dev run "dir" -s cmd

# Auto-detect shell
agent dev run "ls -la"
```

---

## API Server

### Starting the Server

```bash
# Default port (8080)
agent-api

# Custom port
agent-api -port 3000
```

### Endpoints

#### Health Check

```bash
curl http://localhost:8080/health
```

Response:
```json
{
  "status": "ok",
  "timestamp": "2026-02-16T00:02:01-07:00"
}
```

#### Cost Endpoints

```bash
# Current costs
curl http://localhost:8080/api/v1/cost/azure/current

# Cost summary
curl "http://localhost:8080/api/v1/cost/azure/summary?start_date=2026-01-01&end_date=2026-02-01"

# History
curl "http://localhost:8080/api/v1/cost/azure/history?days=30"

# Forecast
curl http://localhost:8080/api/v1/cost/azure/forecast

# Trend
curl http://localhost:8080/api/v1/cost/azure/trend

# All providers
curl http://localhost:8080/api/v1/cost/all

# Report
curl http://localhost:8080/api/v1/cost/report
```

#### Alert Endpoints

```bash
# List alerts
curl http://localhost:8080/api/v1/alerts

# Create alert
curl -X POST http://localhost:8080/api/v1/alerts \
  -H "Content-Type: application/json" \
  -d '{"name": "monthly-budget", "threshold": 100}'

# Check alerts
curl http://localhost:8080/api/v1/alerts/check

# Delete alert
curl -X DELETE "http://localhost:8080/api/v1/alerts?name=monthly-budget"
```

#### Config

```bash
curl http://localhost:8080/api/v1/config
```

---

## Examples

### Daily Cost Check

```bash
#!/bin/bash
# Daily cost check script

echo "=== Azure Costs ==="
agent cost current

echo ""
echo "=== Alerts ==="
agent cost alert check
```

### Cost Report Automation

```bash
# Generate weekly report
agent cost report -o json > weekly-report-$(date +%Y-%m-%d).json
```

### Multi-Cloud Dashboard

```bash
# Get all cloud costs
agent cloud all
```

---

## Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| Azure costs show $0.00 | Wait 24-48 hours for billing data, or check subscription permissions |
| `az login` not working | Run `az login` separately first |
| Ollama not connecting | Ensure Ollama is running (`ollama serve`) |
| API server won't start | Check port is not in use |

### Debug Mode

```bash
# Enable verbose output (if supported)
agent --verbose cost current
```
