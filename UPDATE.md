# Package Manager Update Guide

Tagged releases update the Scoop bucket and the Homebrew tap. GoReleaser writes `bucket/azguard.json` in `cdobratz/scoop-azguard` (branch `master`) and `Casks/azguard.rb` in `cdobratz/homebrew-azguard` (branch `main`). Those files are not stored in this repository.

## Actions secret

Create a fine-grained personal access token that can only access `cdobratz/homebrew-azguard` and `cdobratz/scoop-azguard`. Grant Contents read and write on those two repositories. Save it as the Actions secret `TAP_GITHUB_TOKEN` on `cdobratz/AzGuard`.

The Release workflow passes that secret to GoReleaser. If the secret is missing or empty, GoReleaser still publishes the GitHub release binaries and skips the tap and bucket commits.

## v1.2.0 is already published

The taps update on the next tag, after this change is on `main` and the secret is set. Do not re-run the v1.2.0 Release workflow.

A re-run is not a way to refresh the taps. GitHub Actions runs the workflow file from the tag commit, and v1.2.0 does not contain this GoReleaser config or `TAP_GITHUB_TOKEN`. The v1.2.0 GitHub release also already has its assets. This config leaves `release.replace_existing_artifacts` unset, so a re-run that did build those same asset names would fail with GitHub's `422 already_exists` and would stop before any tap commit. `release.mode` only controls release notes (the default is `keep-existing`). It does not replace assets and it does not make a re-run safe.

## What the release writes

Scoop manifest name and path stay `bucket/azguard.json`. The app name stays `azguard`, the binary stays `azguard.exe`, the license stays MIT, and the homepage is `https://azguard.dev`. GoReleaser puts `bin` on each architecture entry and writes the url and hash itself. It does not write `checkver` or `autoupdate`. `scoop update` still sees a new version because the manifest `version` in the bucket git repo changes.

Homebrew keeps the name `azguard`. Current GoReleaser publishes a cask, not a formula, so the file is `Casks/azguard.rb` rather than `Formula/azguard.rb`. The cask installs the same `azguard` binary (`binary "azguard"`, the same role as the formula's `bin.install "azguard"`). The first release that includes this config adds the cask and does not delete `Formula/azguard.rb`. While both exist, `brew install cdobratz/azguard/azguard` still installs the old formula.

After that release succeeds, in `cdobratz/homebrew-azguard` add `tap_migrations.json` at the repository root:

```json
{
  "azguard": "azguard"
}
```

Then delete `Formula/azguard.rb`. Homebrew uses that file to move an existing formula install onto the cask during `brew upgrade`. New installs then use:

```bash
brew install --cask cdobratz/azguard/azguard
```

The cask has no `license` line. Homebrew casks do not support that stanza, so GoReleaser accepts `license: MIT` and does not write it. The tap's MIT `LICENSE` file is unchanged. The cask also has no formula `test` block. On macOS it clears the quarantine bit on the unsigned binary during install, which is what lets the binary run.

## Release process

```bash
git tag -a v1.0.x -m "Release v1.0.x - Description"
git push origin v1.0.x
```

GoReleaser builds the archives, writes `checksums.txt`, publishes the GitHub release, and commits the cask and the Scoop manifest when `TAP_GITHUB_TOKEN` is set. Commit author is `azguard-releaser <noreply@azguard.dev>`.

## Manual fallback

Use this only when the secret was missing and the release published binaries without updating the taps. Download `checksums.txt` for that tag and edit `bucket/azguard.json` in `cdobratz/scoop-azguard` and `Casks/azguard.rb` in `cdobratz/homebrew-azguard` so the urls and hashes match the release. Push `master` for Scoop and `main` for Homebrew.

## Verification

### Scoop (Windows)

```powershell
scoop update
scoop update azguard
azguard --version
```

### Homebrew (macOS and Linux)

```bash
brew update
brew upgrade --cask azguard
azguard --version
```

Until `Formula/azguard.rb` is removed, `brew upgrade azguard` still follows the formula.

### Wrong hash

Scoop reports "Hash check failed". Homebrew reports "SHA256 mismatch". Copy the hash from `checksums.txt` for that platform. For Scoop, `scoop cache rm azguard` then `scoop update azguard`. For Homebrew, `brew update` then upgrade again.
