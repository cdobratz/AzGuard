# Package Manager Update Guide

This guide explains how to update azguard in Scoop and Homebrew after releasing a new version.

## Overview

When a new version of azguard is released, you need to update two package manager repositories:
1. **Scoop bucket** (scoop-azguard) - For Windows users
2. **Homebrew tap** (homebrew-azguard) - For macOS and Linux users

## Prerequisites

- New release published on GitHub with binaries (done automatically by GoReleaser)
- Access to the scoop-azguard and homebrew-azguard repositories

## Release Process

### 1. Create a New Release

The release process is automated via GitHub Actions:

```bash
# Tag the new version
git tag -a v1.0.x -m "Release v1.0.x - Description"

# Push the tag (this triggers GoReleaser)
git push origin v1.0.x
```

GoReleaser will:
- Build binaries for all platforms (Windows, macOS, Linux)
- Generate checksums.txt
- Create a GitHub release

### 2. Get the Checksums

After the release completes, download the checksums:

```bash
curl -sL https://github.com/cdobratz/AzGuard/releases/download/vX.Y.Z/checksums.txt
```

You'll see output like:
```
<hash>  azguard_X.Y.Z_darwin_amd64.zip
<hash>  azguard_X.Y.Z_darwin_arm64.zip
<hash>  azguard_X.Y.Z_linux_amd64.tar.gz
<hash>  azguard_X.Y.Z_linux_arm64.tar.gz
<hash>  azguard_X.Y.Z_windows_amd64.zip
<hash>  azguard_X.Y.Z_windows_arm64.zip
```

## Update Scoop Bucket

### Location
Repository: https://github.com/cdobratz/scoop-azguard  
File: `bucket/azguard.json`

### Steps

1. **Clone/pull the repository:**
   ```bash
   git clone https://github.com/cdobratz/scoop-azguard.git
   cd scoop-azguard
   ```

2. **Update the manifest:**
   Edit `bucket/azguard.json` and update:
   - `version` field
   - `url` in the 64bit architecture section
   - `hash` with the SHA256 from checksums.txt (windows_amd64)

   ```json
   {
     "version": "X.Y.Z",
     "description": "...",
     "homepage": "...",
     "license": "MIT",
     "architecture": {
       "64bit": {
         "url": "https://github.com/cdobratz/AzGuard/releases/download/vX.Y.Z/azguard_X.Y.Z_windows_amd64.zip",
         "hash": "sha256:<WINDOWS_AMD64_HASH>"
       }
     },
     ...
   }
   ```

3. **Commit and push:**
   ```bash
   git add bucket/azguard.json
   git commit -m "Update azguard to vX.Y.Z"
   git push origin master
   ```

### Testing Scoop Update

Windows users can now update:
```powershell
scoop update azguard
```

## Update Homebrew Tap

### Location
Repository: https://github.com/cdobratz/homebrew-azguard  
File: `Formula/azguard.rb`

### Steps

1. **Clone/pull the repository:**
   ```bash
   git clone https://github.com/cdobratz/homebrew-azguard.git
   cd homebrew-azguard
   ```

2. **Update the formula:**
   Edit `Formula/azguard.rb` and update ALL of the following:
   - `version` field
   - URLs for all platforms (darwin_arm64, darwin_amd64, linux_arm64, linux_amd64)
   - SHA256 hashes for all platforms

   ```ruby
   class Azguard < Formula
     desc "One command to make sure your Azure free tier doesn't surprise you with a bill"
     homepage "https://github.com/cdobratz/AzGuard"
     license "MIT"
     version "X.Y.Z"

     on_macos do
       if Hardware::CPU.arm?
         url "https://github.com/cdobratz/AzGuard/releases/download/vX.Y.Z/azguard_X.Y.Z_darwin_arm64.zip"
         sha256 "<DARWIN_ARM64_HASH>"
       else
         url "https://github.com/cdobratz/AzGuard/releases/download/vX.Y.Z/azguard_X.Y.Z_darwin_amd64.zip"
         sha256 "<DARWIN_AMD64_HASH>"
       end
     end

     on_linux do
       if Hardware::CPU.arm?
         url "https://github.com/cdobratz/AzGuard/releases/download/vX.Y.Z/azguard_X.Y.Z_linux_arm64.tar.gz"
         sha256 "<LINUX_ARM64_HASH>"
       else
         url "https://github.com/cdobratz/AzGuard/releases/download/vX.Y.Z/azguard_X.Y.Z_linux_amd64.tar.gz"
         sha256 "<LINUX_AMD64_HASH>"
       end
     end

     def install
       bin.install "azguard"
     end

     test do
       system "#{bin}/azguard", "--version"
     end
   end
   ```

3. **Commit and push:**
   ```bash
   git add Formula/azguard.rb
   git commit -m "Update azguard to vX.Y.Z"
   git push origin main
   ```

### Testing Homebrew Update

macOS and Linux users can now update:
```bash
brew update
brew upgrade azguard
```

## Quick Reference Script

Here's a complete script to update both package managers:

```bash
#!/bin/bash
VERSION="1.0.4"  # Update this
TAG="v${VERSION}"

# Get checksums
echo "Fetching checksums for ${TAG}..."
CHECKSUMS=$(curl -sL "https://github.com/cdobratz/AzGuard/releases/download/${TAG}/checksums.txt")

# Extract individual hashes
DARWIN_AMD64=$(echo "$CHECKSUMS" | grep darwin_amd64 | awk '{print $1}')
DARWIN_ARM64=$(echo "$CHECKSUMS" | grep darwin_arm64 | awk '{print $1}')
LINUX_AMD64=$(echo "$CHECKSUMS" | grep linux_amd64 | awk '{print $1}')
LINUX_ARM64=$(echo "$CHECKSUMS" | grep linux_arm64 | awk '{print $1}')
WINDOWS_AMD64=$(echo "$CHECKSUMS" | grep windows_amd64 | awk '{print $1}')

echo "Darwin amd64:  $DARWIN_AMD64"
echo "Darwin arm64:  $DARWIN_ARM64"
echo "Linux amd64:   $LINUX_AMD64"
echo "Linux arm64:   $LINUX_ARM64"
echo "Windows amd64: $WINDOWS_AMD64"

# Update Scoop
echo -e "\nUpdate Scoop manifest at: scoop-azguard/bucket/azguard.json"
echo "  - version: ${VERSION}"
echo "  - url: ...${TAG}/azguard_${VERSION}_windows_amd64.zip"
echo "  - hash: sha256:${WINDOWS_AMD64}"

# Update Homebrew
echo -e "\nUpdate Homebrew formula at: homebrew-azguard/Formula/azguard.rb"
echo "  - version: ${VERSION}"
echo "  - darwin_arm64 hash: ${DARWIN_ARM64}"
echo "  - darwin_amd64 hash: ${DARWIN_AMD64}"
echo "  - linux_arm64 hash:  ${LINUX_ARM64}"
echo "  - linux_amd64 hash:  ${LINUX_AMD64}"
```

## Verification

After updating both package managers, verify users can install/update:

### Scoop (Windows)
```powershell
scoop update
scoop update azguard
azguard --version
```

### Homebrew (macOS/Linux)
```bash
brew update
brew upgrade azguard
azguard --version
```

Both should show the new version number.

## Troubleshooting

### Wrong Hash Error
- **Scoop**: "Hash check failed"
- **Homebrew**: "SHA256 mismatch"

**Solution**: Double-check you copied the correct hash from checksums.txt for the right platform/architecture.

### Old Version Still Installing
- **Scoop**: Run `scoop cache rm azguard` then `scoop update azguard`
- **Homebrew**: Run `brew update` to refresh the tap, then `brew upgrade azguard`

### Formula/Manifest Syntax Error
- Validate JSON for Scoop: Use a JSON linter
- Validate Ruby for Homebrew: Run `brew audit --strict azguard`

## Automation Opportunities

Consider automating this process with:
1. GitHub Actions workflow that triggers on new releases
2. Script that automatically updates both repositories
3. Bot that creates PRs to package manager repos

## Notes

- Always update both Scoop and Homebrew together to keep versions in sync
- Test on at least one platform before pushing
- Include release notes in commit messages
- The autoupdate section in Scoop manifest helps users get notified of updates
