# Package Manager Update Guide

Tagged releases update the Scoop bucket and the Homebrew formula. GoReleaser writes `bucket/azguard.json` in `cdobratz/scoop-azguard` (branch `master`). After that publish succeeds, the Release workflow writes `Formula/azguard.rb` in `cdobratz/homebrew-azguard` (branch `main`). Those files are not stored in this repository.

`brew install cdobratz/azguard/azguard` stays the install command. The formula class, `bin.install "azguard"`, the test block, and the MIT license stay as they are. The release step updates the version, the per-OS urls, and the sha256 values from `checksums.txt`.

## Actions secret

Create a fine-grained personal access token that can only access `cdobratz/homebrew-azguard` and `cdobratz/scoop-azguard`. Grant Contents read and write on those two repositories. Save it as the Actions secret `TAP_GITHUB_TOKEN` on `cdobratz/AzGuard`.

The Release workflow passes that secret to GoReleaser and to the formula step. If the secret is missing or empty, the GitHub release binaries still publish. GoReleaser skips the Scoop commit, and the formula step exits 0 without pushing.

## v1.2.0 is already published

Do not re-run the v1.2.0 Release workflow that built the binaries. That run uses the workflow file from the tag, which does not contain this formula step. The release already has its assets. `release.replace_existing_artifacts` is unset, so uploading those same names again fails with GitHub `422 already_exists`. `release.mode` only controls release notes (the default is `keep-existing`). It does not replace assets.

After this change is on `main` and the secret is saved, update v1.2.0 without GoReleaser: Actions, Release, Run workflow, tag `v1.2.0`. That dispatch downloads the existing `checksums.txt`, commits `Formula/azguard.rb` when it changed, and patches `bucket/azguard.json` (version, urls, and hashes only). It leaves `bin`, `checkver`, `autoupdate`, the description, the homepage, and the license in place. A second run commits nothing if the files already match.

The next `v*` tag updates Scoop through GoReleaser and the formula through the post-release step. GoReleaser's Scoop manifest uses `https://azguard.dev`, puts `bin` on each architecture, and does not write `checkver` or `autoupdate`. The manual v1.2.0 patch does not do that rewrite.

## Release process

```bash
git tag -a v1.0.x -m "Release v1.0.x - Description"
git push origin v1.0.x
```

GoReleaser builds the archives, writes `checksums.txt`, publishes the GitHub release, and commits the Scoop manifest when `TAP_GITHUB_TOKEN` is set. The next step commits the Homebrew formula from that `checksums.txt`. Commit author is `azguard-releaser <noreply@azguard.dev>`. If the formula or manifest is already at that tag, there is no commit.

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
brew upgrade azguard
azguard --version
```

### Wrong hash

Scoop reports "Hash check failed". Homebrew reports "SHA256 mismatch". The hash has to come from `checksums.txt` for that platform. For Scoop, `scoop cache rm azguard` then `scoop update azguard`. For Homebrew, `brew update` then `brew upgrade azguard`.
