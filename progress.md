# azguard Project Progress

## Project Overview

**azguard** - One command to make sure your Azure free tier doesn't surprise you with a bill.

### The Pivot

This project was previously "Agent CLI" - a general-purpose tool for software development and cloud cost management. After strategic analysis, it has been pivoted to focus specifically on **Azure free tier bill protection**.

### Why This Pivot?

- **Clear pain point**: Azure free tier confusion is well-documented
- **Defined audience**: Students, freelancers, bootcamp grads, small dev shops
- **Low competition**: No CLI tool specifically for this use case
- **Viral potential**: Every Azure bill shock forum post is distribution

---

## Completed

### Core Features

| Feature | Status | Description |
|---------|--------|-------------|
| Azure Cost API | ✅ | Query Azure Cost Management API |
| Budget Alerts | ✅ | Set threshold alerts ($1-$100) |
| SQLite Storage | ✅ | Local cost data storage |
| CLI Commands | ✅ | scan, status, budget, cost, resources, cleanup |
| Free Tier Limits | ✅ | Config file with known free tier limits |

### Commands

```
azguard status              Quick overview
azguard scan              Scan for overages  
azguard resources         List resources
azguard budget add 5      Add $5 alert
azguard budget list       List alerts
azguard cost fetch       Fetch costs
azguard cost current      Current costs
azguard cleanup          Cleanup guide
```

---

## Next Steps

### Phase 1: Polish
- [ ] Fix remaining bugs
- [ ] Add tests
- [ ] Clean up code

### Phase 2: Distribution
- [ ] Set up GitHub repo
- [ ] Configure GoReleaser
- [ ] Add to Homebrew
- [ ] Add to Scoop
- [ ] Create install script (azguard.dev)

### Phase 3: Growth
- [ ] Blog post launch
- [ ] Reddit/Azure community posts
- [ ] Submit to awesome lists

### Phase 4: Features
- [ ] Watch mode (continuous monitoring)
- [ ] Slack/Teams notifications
- [ ] AWS free tier guard
- [ ] GCP free tier guard
- [ ] Web dashboard

---

## Files Structure

```
azguard/
├── cmd/agent/main.go       # CLI entry
├── internal/
│   ├── config/             # Configuration
│   ├── storage/            # SQLite
│   ├── cloud/azure/         # Azure integration
│   └── cost/                # Cost service + free tier
├── configs/
│   └── free_tier_limits.yaml
├── README.md
├── USAGE.md
├── CONTRIBUTING.md
└── LICENSE
```

---

## Known Issues

- $0.00 costs may show if billing data not yet available (24-48 hour delay)
- Need Cost Management Reader role for full functionality

---

## Links

- GitHub: (to be created)
- Website: https://azguard.dev (to be created)
