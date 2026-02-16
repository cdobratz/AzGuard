# Implementation Plan: Distribution & Deployment

## Overview

| Phase | Task | Priority | Effort |
|-------|------|----------|--------|
| 1 | GitHub Releases with GoReleaser | High | Medium |
| 2 | Windows: Scoop + Chocolatey | High | Low |
| 3 | Homebrew Tap | Medium | Low |

---

## Phase 1: GitHub Releases with GoReleaser

### Prerequisites
- GitHub repository created
- GitHub Personal Access Token (PAT) with `repo` scope

### Steps

#### 1. Install GoReleaser
```bash
curl -sL https://git.io/goreleaser | bash
```

#### 2. Create `.goreleaser.yaml` config
```yaml
project_name: agent
builds:
  - id: agent-cli
    binary: agent
    main: ./cmd/agent/main.go
    env:
      - CGO_ENABLED=0
    goos:
      - windows
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
  - id: agent-api
    binary: agent-api
    main: ./cmd/api/main.go
    env:
      - CGO_ENABLED=0
    goos:
      - windows
      - linux
      - darwin
    goarch:
      - amd64
archives:
  - id: default
    format: zip
    format_overrides:
      - goos: linux
        format: tar.gz
release:
  github:
    owner: YOUR_GITHUB_USER
    name: agent
  draft: false
  prerelease: auto
```

#### 3. Add to CI/CD (GitHub Actions)
Create `.github/workflows/release.yml`:

```yaml
name: Release
on:
  push:
    tags:
      - 'v*'
jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - uses: goreleaser/goreleaser-action@v5
        with:
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

#### 4. Release Process
```bash
git tag v1.0.0
git push origin v1.0.0
```

### Update Strategy: Manual
- Users download new version manually from GitHub Releases
- No additional infrastructure needed
- User controls when to update

---

## Phase 2: Windows Package Managers

### Option A: Scoop

#### 1. Create Bucket Repository
```bash
scoop bucket create agent
cd agent
```

#### 2. Add Manifest
Create `bucket/agent.json`:

```json
{
  "version": "1.0.0",
  "description": "CLI for software development and cloud cost management",
  "homepage": "https://github.com/YOUR_USER/agent",
  "license": "MIT",
  "architecture": {
    "64bit": {
      "url": "https://github.com/YOUR_USER/agent/releases/download/v1.0.0/agent-windows-amd64.zip",
      "hash": "sha256:..."
    }
  },
  "bin": "agent.exe",
  "checkver": {
    "github": "https://github.com/YOUR_USER/agent"
  },
  "autoupdate": {
    "architecture": {
      "64bit": {
        "url": "https://github.com/YOUR_USER/agent/releases/download/v$version/agent-windows-amd64.zip"
      }
    }
  }
}
```

#### 3. Users Install
```powershell
scoop bucket add extras
scoop install agent
```

### Option B: Chocolatey

#### 1. Create Package
Create `tools/chocolatey/agent.nuspec`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://schemas.microsoft.com/packaging/2010/07/nuspec.xsd">
  <metadata>
    <id>agent</id>
    <version>1.0.0</version>
    <title>Agent CLI</title>
    <authors>Your Name</authors>
    <description>CLI for software development and cloud cost management</description>
    <projectUrl>https://github.com/YOUR_USER/agent</projectUrl>
  </metadata>
  <files>
    <file src="tools\*" target="tools" />
  </files>
</package>
```

#### 2. Users Install
```powershell
choco install agent
```

---

## Phase 3: Homebrew (macOS/Linux)

### Option A: Personal Tap

#### 1. Create Formula
Create `Formula/agent.rb`:

```ruby
class Agent < Formula
  desc "CLI for software development and cloud cost management"
  homepage "https://github.com/YOUR_USER/agent"
  version "1.0.0"
  license "MIT"
  
  url "https://github.com/YOUR_USER/agent/archive/refs/tags/v1.0.0.tar.gz"
  sha256 "..."
  
  def install
    bin.install "agent"
    bin.install "agent-api"
  end
  
  test do
    system "#{bin}/agent", "--version"
  end
end
```

#### 2. Users Install
```bash
brew install YOUR_USER/homebrew-agent/agent
```

### Option B: Submit to Homebrew Core

#### 1. Create PR to Homebrew/homebrew-core
- Fork repository
- Add formula: `Formula/agent.rb`
- Follow Homebrew style guidelines

#### 2. Users Install
```bash
brew install agent
```

---

## Files to Create

| File | Purpose |
|------|---------|
| `.goreleaser.yaml` | Build configuration |
| `.github/workflows/release.yml` | CI/CD pipeline |
| `bucket/agent.json` | Scoop manifest |
| `tools/chocolatey/agent.nuspec` | Chocolatey package |
| `Formula/agent.rb` | Homebrew formula |

---

## Manual vs Package Manager Updates

| Aspect | Manual (GitHub) | Package Manager |
|--------|-----------------|-----------------|
| **Security** | User verifies | Package manager verifies |
| **Cost** | Free | Free |
| **Convenience** | Manual download | `brew/scoop update` |
| **Infrastructure** | GitHub only | Package repo required |

**Recommendation**: Both are secure. Package managers are more convenient and handle updates automatically.
