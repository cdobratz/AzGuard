#!/usr/bin/env bash
# Download AzGuard, price a Terraform plan, optionally list idle resources,
# and post or update one pull request comment.
set -euo pipefail

install_azguard() {
  local version="$1"
  local dest="$2"
  local os arch tag ver ext asset url
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    linux|darwin) ;;
    mingw*|msys*|cygwin*) os=windows ;;
    *)
      echo "unsupported OS: $os" >&2
      return 1
      ;;
  esac
  arch=$(uname -m)
  case "$arch" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *)
      echo "unsupported architecture: $arch" >&2
      return 1
      ;;
  esac
  if [ "$version" = "latest" ]; then
    tag=$(curl -fsSL "https://api.github.com/repos/cdobratz/AzGuard/releases/latest" | python3 -c 'import json,sys; print(json.load(sys.stdin)["tag_name"])')
  else
    tag="$version"
    case "$tag" in
      v*) ;;
      *) tag="v${tag}" ;;
    esac
  fi
  ver="${tag#v}"
  if [ "$os" = "linux" ]; then
    ext="tar.gz"
  else
    ext="zip"
  fi
  asset="azguard_${ver}_${os}_${arch}.${ext}"
  url="https://github.com/cdobratz/AzGuard/releases/download/${tag}/${asset}"
  mkdir -p "$dest"
  curl -fsSL -o "$dest/$asset" "$url"
  curl -fsSL -o "$dest/checksums.txt" "https://github.com/cdobratz/AzGuard/releases/download/${tag}/checksums.txt"
  verify_checksum "$dest/$asset" "$dest/checksums.txt"
  if [ "$ext" = "tar.gz" ]; then
    tar -xzf "$dest/$asset" -C "$dest"
  else
    unzip -qo "$dest/$asset" -d "$dest"
  fi
  if [ "$os" = "windows" ]; then
    printf '%s\n' "$dest/azguard.exe"
  else
    printf '%s\n' "$dest/azguard"
  fi
}

verify_checksum() {
  local file="$1"
  local sums="$2"
  local name line
  name=$(basename "$file")
  line=$(grep -E "^[0-9a-fA-F]+[[:space:]]+${name}$" "$sums" || true)
  if [ -z "$line" ]; then
    echo "checksums.txt has no entry for ${name}" >&2
    return 1
  fi
  (
    cd "$(dirname "$file")"
    printf '%s\n' "$line" | sha256sum -c --status -
  )
}

post_comment() {
  local file="$1"
  local marker="<!-- azguard-report -->"
  local existing payload
  if [ -z "${GITHUB_REPOSITORY:-}" ] || [ -z "${PR_NUMBER:-}" ]; then
    echo "GITHUB_REPOSITORY and PR_NUMBER are required to post a comment." >&2
    return 1
  fi
  if [ -z "${GITHUB_TOKEN:-}" ] && [ -z "${GH_TOKEN:-}" ]; then
    echo "github-token is required to post a pull request comment." >&2
    return 1
  fi
  payload=$(mktemp)
  python3 -c 'import json,sys; json.dump({"body": open(sys.argv[1], encoding="utf-8").read()}, sys.stdout)' "$file" > "$payload"
  existing=$(gh api "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments" --paginate --jq ".[] | select(.body | contains(\"${marker}\")) | .id" | head -n 1 || true)
  if [ -n "$existing" ]; then
    gh api --method PATCH "repos/${GITHUB_REPOSITORY}/issues/comments/${existing}" --input "$payload" >/dev/null
  else
    gh api --method POST "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments" --input "$payload" >/dev/null
  fi
  rm -f "$payload"
}

main() {
  local plan budget version scan comment bin plan_json idle_json plan_note idle_note exceeded
  plan="${INPUT_PLAN:-plan.json}"
  budget="${INPUT_BUDGET:-}"
  version="${INPUT_VERSION:-latest}"
  scan="${INPUT_SCAN_IDLE:-false}"
  comment="${INPUT_COMMENT:-true}"
  WORK_DIR=$(mktemp -d)
  trap 'rm -rf "$WORK_DIR"' EXIT

  if [ -n "${AZGUARD_BIN:-}" ]; then
    bin="$AZGUARD_BIN"
  else
    bin=$(install_azguard "$version" "$WORK_DIR/bin")
  fi

  plan_json=""
  plan_note=""
  if [ -f "$plan" ]; then
    plan_json="$WORK_DIR/preflight.json"
    "$bin" preflight --plan "$plan" > "$plan_json"
  else
    plan_note="Plan file \`$plan\` was not found, so there is no preflight estimate."
  fi

  idle_json=""
  idle_note=""
  if [ "$scan" = "true" ]; then
    idle_json="$WORK_DIR/idle.json"
    set +e
    "$bin" resources --output json > "$idle_json" 2>"$WORK_DIR/idle.err"
    local idle_status=$?
    set -e
    if [ "$idle_status" -ne 0 ]; then
      if grep -q "not configured" "$idle_json" "$WORK_DIR/idle.err" 2>/dev/null; then
        idle_note="Idle scan skipped because cloud credentials are not configured."
        idle_json=""
      elif ! python3 -c 'import json,sys; json.load(open(sys.argv[1], encoding="utf-8"))' "$idle_json" 2>/dev/null; then
        idle_note="Idle scan failed. $(tr '\n' ' ' < "$WORK_DIR/idle.err")"
        idle_json=""
      fi
    fi
  fi

  python3 - "$plan_json" "$idle_json" "$plan_note" "$idle_note" "$budget" "$WORK_DIR/comment.md" "$WORK_DIR/result.json" <<'PY'
import json
import sys

plan_path, idle_path, plan_note, idle_note, budget, comment_path, result_path = sys.argv[1:8]

def load(path):
    if not path:
        return None
    with open(path, encoding="utf-8") as handle:
        return json.load(handle)

plan = load(plan_path)
idle = load(idle_path)
monthly = 0.0
priced = False
billable = False
message = plan_note or "No Terraform plan was checked."
items = []
if plan:
    monthly = float(plan.get("monthly_usd") or 0)
    priced = bool(plan.get("priced"))
    billable = bool(plan.get("billable"))
    message = plan.get("message") or message
    items = plan.get("items") or []

idle_total = None
findings = []
if idle:
    idle_total = float(idle.get("total_estimated_monthly_usd") or 0)
    findings = idle.get("findings") or []
    errors = idle.get("errors") or []
    if not findings and any("not configured" in str(err) for err in errors):
        idle_note = idle_note or "Idle scan skipped because cloud credentials are not configured."
        findings = []
        idle_total = None

spend = None
if plan and billable:
    spend = monthly
elif idle_total is not None:
    spend = idle_total

exceeded = False
budget_value = None
if budget.strip():
    budget_value = float(budget)
    if plan and billable and not priced:
        exceeded = True
    elif spend is not None and spend > budget_value:
        exceeded = True

lines = ["<!-- azguard-report -->", "## AzGuard", "", message, ""]
if plan and items:
    lines.append("### Resources the plan would create")
    lines.append("")
    for item in items:
        label = item.get("label") or item.get("address") or "resource"
        if item.get("priced"):
            lines.append("- %s: about $%.2f/month" % (label, float(item.get("monthly_usd") or 0)))
        else:
            lines.append("- %s: no bundled price" % label)
    lines.append("")
    if priced:
        lines.append("Estimated monthly total: $%.2f" % monthly)
    else:
        lines.append("The estimate is incomplete, so the total is not a full monthly price.")
    lines.append("")
if findings:
    lines.append("### Idle resources")
    lines.append("")
    for finding in findings:
        name = finding.get("name") or finding.get("resource_id") or "resource"
        rule = finding.get("rule_id") or ""
        lines.append("- %s %s: about $%.2f/month" % (rule, name, float(finding.get("estimated_monthly_usd") or 0)))
    lines.append("")
    lines.append("Idle estimated monthly total: $%.2f" % idle_total)
    lines.append("")
elif idle_note:
    lines.append(idle_note)
    lines.append("")
if budget_value is not None:
    lines.append("Budget: $%.2f/month" % budget_value)
    if exceeded:
        lines.append("Result: estimated spend exceeds the budget.")
    else:
        lines.append("Result: estimated spend is within the budget.")
    lines.append("")
lines.append("Prices are approximate list prices from AzGuard's bundled table (730 hours per month), not an invoice. For a full infrastructure cost estimate, use [Infracost](https://www.infracost.io/).")
lines.append("")
text = "\n".join(lines)
with open(comment_path, "w", encoding="utf-8") as handle:
    handle.write(text)
with open(result_path, "w", encoding="utf-8") as handle:
    json.dump({"exceeded": exceeded, "monthly_usd": monthly if plan else 0}, handle)
print(text)
PY

  if [ -n "${AZGUARD_COMMENT_OUT:-}" ]; then
    cp "$WORK_DIR/comment.md" "$AZGUARD_COMMENT_OUT"
  elif [ "$comment" = "true" ] && [ "${GITHUB_EVENT_NAME:-}" = "pull_request" ] && [ -n "${PR_NUMBER:-}" ]; then
    post_comment "$WORK_DIR/comment.md"
  fi

  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    python3 -c 'import json,sys; data=json.load(open(sys.argv[1], encoding="utf-8")); print("monthly_usd=%s" % data["monthly_usd"]); print("exceeded=%s" % ("true" if data["exceeded"] else "false"))' "$WORK_DIR/result.json" >> "$GITHUB_OUTPUT"
  fi

  exceeded=$(python3 -c 'import json,sys; print("1" if json.load(open(sys.argv[1], encoding="utf-8"))["exceeded"] else "0")' "$WORK_DIR/result.json")
  if [ "$exceeded" = "1" ]; then
    echo "AzGuard: estimated spend exceeds the budget." >&2
    exit 1
  fi
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
