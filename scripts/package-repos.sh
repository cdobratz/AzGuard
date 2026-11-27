#!/usr/bin/env bash
# Render and publish the Homebrew formula, and patch the Scoop manifest.
# An empty TAP_GITHUB_TOKEN skips the git push and exits 0.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO="https://github.com/cdobratz/AzGuard"
FORMULA_REPO="https://github.com/cdobratz/homebrew-azguard.git"
SCOOP_REPO="https://github.com/cdobratz/scoop-azguard.git"
AUTHOR_NAME="azguard-releaser"
AUTHOR_EMAIL="noreply@azguard.dev"

usage() {
  cat <<'EOF'
Usage: scripts/package-repos.sh <command> [options]

Commands:
  render-formula --tag v1.2.0 --checksums checksums.txt
  publish --tag v1.2.0 [--checksums checksums.txt] [--formula] [--scoop]

publish writes Formula/azguard.rb on homebrew-azguard main and, with --scoop,
patches bucket/azguard.json on scoop-azguard master. It commits only when the
file changed. With an empty TAP_GITHUB_TOKEN it prints a skip message and
exits 0.
EOF
}

die() {
  echo "package-repos: $*" >&2
  exit 1
}

require_tag() {
  local tag="$1"
  if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.+-][0-9A-Za-z.+-]+)?$ ]]; then
    die "tag must look like v1.2.0, got ${tag}"
  fi
}

version_of() {
  echo "${1#v}"
}

checksum_for() {
  local file="$1"
  local name="$2"
  local digest
  digest="$(awk -v name="$name" '$NF == name { print $1; found = 1 } END { if (!found) exit 1 }' "$file" || true)"
  if [[ ! "$digest" =~ ^[0-9a-f]{64}$ ]]; then
    die "checksums.txt has no usable sha256 for ${name}"
  fi
  printf '%s\n' "$digest"
}

render_formula() {
  local tag="$1"
  local checksums="$2"
  local ver darwin_arm64 darwin_amd64 linux_arm64 linux_amd64
  require_tag "$tag"
  ver="$(version_of "$tag")"
  darwin_arm64="$(checksum_for "$checksums" "azguard_${ver}_darwin_arm64.zip")"
  darwin_amd64="$(checksum_for "$checksums" "azguard_${ver}_darwin_amd64.zip")"
  linux_arm64="$(checksum_for "$checksums" "azguard_${ver}_linux_arm64.tar.gz")"
  linux_amd64="$(checksum_for "$checksums" "azguard_${ver}_linux_amd64.tar.gz")"

  cat <<EOF
class Azguard < Formula
  desc "One command so Azure and AWS free tiers don't surprise you with a bill"
  homepage "https://github.com/cdobratz/AzGuard"
  license "MIT"
  version "${ver}"

  on_macos do
    if Hardware::CPU.arm?
      url "${REPO}/releases/download/${tag}/azguard_${ver}_darwin_arm64.zip"
      sha256 "${darwin_arm64}"
    else
      url "${REPO}/releases/download/${tag}/azguard_${ver}_darwin_amd64.zip"
      sha256 "${darwin_amd64}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "${REPO}/releases/download/${tag}/azguard_${ver}_linux_arm64.tar.gz"
      sha256 "${linux_arm64}"
    else
      url "${REPO}/releases/download/${tag}/azguard_${ver}_linux_amd64.tar.gz"
      sha256 "${linux_amd64}"
    end
  end

  def install
    bin.install "azguard"
  end

  test do
    system bin/"azguard", "--version"
  end
end
EOF
}

download_checksums() {
  local tag="$1"
  local dest="$2"
  curl -fsSL "${REPO}/releases/download/${tag}/checksums.txt" -o "$dest"
}

# GitHub's git HTTPS endpoint accepts a PAT as Basic auth, not Bearer.
# The header is passed with git -c so it is not stored in .git/config or the remote URL.
git_basic_header() {
  printf 'AUTHORIZATION: basic %s' "$(printf 'x-access-token:%s' "$TAP_GITHUB_TOKEN" | base64 | tr -d '\n')"
}

with_auth() {
  GIT_TERMINAL_PROMPT=0 git -c "http.extraHeader=$(git_basic_header)" "$@"
}

commit_if_changed() {
  local dir="$1"
  local path="$2"
  local message="$3"
  local branch="$4"
  if git -C "$dir" diff --quiet -- "$path"; then
    echo "unchanged ${path}"
    return 0
  fi
  git -C "$dir" add -- "$path"
  git -C "$dir" -c "user.name=${AUTHOR_NAME}" -c "user.email=${AUTHOR_EMAIL}" commit -m "$message"
  with_auth -C "$dir" push origin "HEAD:${branch}"
  echo "pushed ${path} to ${branch}"
}

clone_repo() {
  local url="$1"
  local branch="$2"
  local dest="$3"
  with_auth clone --depth 1 --branch "$branch" "$url" "$dest"
}

publish() {
  local tag="" checksums="" do_formula=0 do_scoop=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --tag)
        tag="$2"
        shift 2
        ;;
      --checksums)
        checksums="$2"
        shift 2
        ;;
      --formula)
        do_formula=1
        shift
        ;;
      --scoop)
        do_scoop=1
        shift
        ;;
      *)
        die "unknown option $1"
        ;;
    esac
  done
  [[ -n "$tag" ]] || die "--tag is required"
  require_tag "$tag"
  if [[ "$do_formula" -eq 0 && "$do_scoop" -eq 0 ]]; then
    die "pass --formula, --scoop, or both"
  fi
  if [[ -z "${TAP_GITHUB_TOKEN:-}" ]]; then
    echo "TAP_GITHUB_TOKEN is empty; skipping Homebrew and Scoop updates"
    exit 0
  fi
  # Fail immediately if git would otherwise prompt for a username.
  export GIT_TERMINAL_PROMPT=0

  local work sums
  work="$(mktemp -d)"
  # RETURN traps are not function-scoped. Clear this one after it runs so a
  # later return does not expand work after it has gone out of scope.
  trap 'rm -rf "${work:-}"; trap - RETURN' RETURN
  if [[ -n "$checksums" ]]; then
    sums="$checksums"
  else
    sums="${work}/checksums.txt"
    download_checksums "$tag" "$sums"
  fi

  if [[ "$do_formula" -eq 1 ]]; then
    clone_repo "$FORMULA_REPO" main "${work}/homebrew"
    render_formula "$tag" "$sums" > "${work}/homebrew/Formula/azguard.rb"
    commit_if_changed "${work}/homebrew" "Formula/azguard.rb" "Update azguard formula to ${tag}" main
  fi
  if [[ "$do_scoop" -eq 1 ]]; then
    clone_repo "$SCOOP_REPO" master "${work}/scoop"
    local status
    status="$(python3 "${ROOT}/scripts/update_scoop_manifest.py" \
      --tag "$tag" \
      --checksums "$sums" \
      --manifest "${work}/scoop/bucket/azguard.json")"
    echo "scoop ${status}"
    if [[ "$status" == "updated" ]]; then
      commit_if_changed "${work}/scoop" "bucket/azguard.json" "Update azguard manifest to ${tag}" master
    else
      echo "unchanged bucket/azguard.json"
    fi
  fi
}

main() {
  local cmd="${1:-}"
  if [[ -z "$cmd" ]]; then
    usage
    exit 2
  fi
  shift
  case "$cmd" in
    render-formula)
      local tag="" checksums=""
      while [[ $# -gt 0 ]]; do
        case "$1" in
          --tag)
            tag="$2"
            shift 2
            ;;
          --checksums)
            checksums="$2"
            shift 2
            ;;
          *)
            die "unknown option $1"
            ;;
        esac
      done
      [[ -n "$tag" && -n "$checksums" ]] || die "render-formula needs --tag and --checksums"
      render_formula "$tag" "$checksums"
      ;;
    publish)
      publish "$@"
      ;;
    -h|--help|help)
      usage
      ;;
    *)
      usage
      exit 2
      ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
