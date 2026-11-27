#!/usr/bin/env bash
# Offline checks for the Homebrew formula renderer and the Scoop manifest patch.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${ROOT}/scripts/package-repos.sh"
PY="${ROOT}/scripts/update_scoop_manifest.py"
DATA="${ROOT}/scripts/testdata"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

"$SCRIPT" render-formula --tag v1.2.0 --checksums "${DATA}/v1.2.0-checksums.txt" > "${tmp}/formula.rb"
diff -u "${DATA}/v1.2.0-formula.rb" "${tmp}/formula.rb"

# The class, install, test, and license stay on the lines the tap already uses.
grep -F 'class Azguard < Formula' "${tmp}/formula.rb" >/dev/null
grep -F 'license "MIT"' "${tmp}/formula.rb" >/dev/null
grep -F 'bin.install "azguard"' "${tmp}/formula.rb" >/dev/null
grep -F 'system bin/"azguard", "--version"' "${tmp}/formula.rb" >/dev/null
grep -F 'version "1.2.0"' "${tmp}/formula.rb" >/dev/null

skip_out="$(TAP_GITHUB_TOKEN= "$SCRIPT" publish --tag v1.2.0 --checksums "${DATA}/v1.2.0-checksums.txt" --formula --scoop)"
[[ "$skip_out" == *"TAP_GITHUB_TOKEN is empty"* ]]

# GitHub git HTTPS wants Basic auth. A Bearer PAT gets 401 and git prompts.
token='test-token'
want="AUTHORIZATION: basic $(printf 'x-access-token:%s' "$token" | base64 | tr -d '\n')"
header="$(TAP_GITHUB_TOKEN="$token" bash -c 'source "$1"; git_basic_header' _ "$SCRIPT")"
[[ "$header" == "$want" ]]
[[ "$header" != *'bearer '* ]]
[[ "$header" != *"$token"* ]]

bindir="${tmp}/bin"
mkdir -p "$bindir"
cat > "${bindir}/git" <<'EOF'
#!/bin/bash
printf 'PROMPT=%s\n' "${GIT_TERMINAL_PROMPT-unset}"
printf '%s\n' "$@"
EOF
chmod +x "${bindir}/git"
recorded="$(PATH="${bindir}:${PATH}" TAP_GITHUB_TOKEN="$token" bash -c 'source "$1"; with_auth status' _ "$SCRIPT")"
[[ "$(printf '%s\n' "$recorded" | head -n 1)" == "PROMPT=0" ]]
grep -F "http.extraHeader=${want}" <<<"$recorded" >/dev/null
grep -F 'bearer ' <<<"$recorded" >/dev/null && exit 1
grep -F "$token" <<<"$recorded" >/dev/null && exit 1
true

cp "${DATA}/scoop-before.json" "${tmp}/azguard.json"
status="$(python3 "$PY" --tag v1.2.0 --checksums "${DATA}/v1.2.0-checksums.txt" --manifest "${tmp}/azguard.json")"
[[ "$status" == "updated" ]]
python3 - "${tmp}/azguard.json" <<'PY'
import json, sys
doc = json.load(open(sys.argv[1]))
assert doc["version"] == "1.2.0"
assert doc["bin"] == "azguard.exe"
assert doc["license"] == "MIT"
assert doc["homepage"] == "https://github.com/cdobratz/AzGuard"
assert doc["description"].startswith("One command to make sure")
assert doc["checkver"]["github"] == "https://github.com/cdobratz/AzGuard"
assert "$version" in doc["autoupdate"]["architecture"]["64bit"]["url"]
amd = doc["architecture"]["64bit"]
arm = doc["architecture"]["arm64"]
assert amd["url"].endswith("/v1.2.0/azguard_1.2.0_windows_amd64.zip")
assert arm["url"].endswith("/v1.2.0/azguard_1.2.0_windows_arm64.zip")
assert amd["hash"] == "sha256:cabd6ead895852cc738871da0cfd6cefea86a4a15d8a27b3994c447bfb06937f"
assert arm["hash"] == "sha256:b0c6d815aaa472c9fc7e6546e4d877a95c5417cda648a152564df21874540f6c"
PY

second="$(python3 "$PY" --tag v1.2.0 --checksums "${DATA}/v1.2.0-checksums.txt" --manifest "${tmp}/azguard.json")"
[[ "$second" == "unchanged" ]]

echo "package-repos tests passed"
