# scoop-azguard

Scoop bucket for [AzGuard](https://github.com/cdobratz/AzGuard) — One command to make sure your Azure free tier doesn't surprise you with a bill.

## Install

```powershell
# Add the bucket
scoop bucket add azguard https://github.com/cdobratz/scoop-azguard

# Install AzGuard
scoop install azguard
```

## Troubleshooting

If azguard doesn't run after install:

1. **Restart your terminal** - This reloads the PATH
2. **Manually add Scoop to PATH:**
   ```powershell
   $env:PATH += ";$env:USERPROFILE\scoop\shims"
   ```
3. **Check installation:**
   ```powershell
   scoop which azguard
   ```

For permanent fix, add the Scoop shims folder to your system PATH.

## Usage

```powershell
# Check your Azure free tier status
azguard status

# Scan resources approaching limits
azguard scan

# Set budget alerts
azguard budget --set 50

# List all resources
azguard resources

# Clean up unused resources
azguard cleanup

# View cost breakdown
azguard cost
```

## Requirements

- Azure subscription
- Azure CLI (`az`) authenticated

For more commands and options, see the [full documentation](https://github.com/cdobratz/AzGuard).

## Other Install Methods

- **Homebrew:** `brew install cdobratz/azguard/azguard`
- **Direct:** Download from [GitHub Releases](https://github.com/cdobratz/AzGuard/releases)

## License

MIT — See [AzGuard](https://github.com/cdobratz/AzGuard) for details.
