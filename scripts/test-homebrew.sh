#!/bin/sh
# Exercise release updates without downloading or installing the app.
# Expected wrapper lines contain literal shell variables.
# shellcheck disable=SC2016
set -eu

root="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
mkdir -p "$tmp/scripts" "$tmp/bin"
cp "$root/scripts/update-homebrew.sh" "$tmp/scripts/"
cp "$root/scripts/fiveleaf.rb.in" "$tmp/scripts/"
cat > "$tmp/bin/gh" <<'SH'
#!/bin/sh
set -eu
while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo) shift; test "$1" = "example/FiveLeaf" ;;
    --dir) shift; dir="$1" ;;
    --pattern) shift; printf '%s\n' "$1" > "$dir/$1" ;;
  esac
  shift
done
SH
chmod +x "$tmp/bin/gh"
PATH="$tmp/bin:$PATH"
export PATH
FIVELEAF_REPOSITORY=example/FiveLeaf
export FIVELEAF_REPOSITORY

# The first release creates the cask using only its own artifacts and repository.
sh "$tmp/scripts/update-homebrew.sh" 0.12.8
grep -Fxq 'cask "fiveleaf" do' "$tmp/Casks/fiveleaf.rb"
grep -Fxq '  app "FiveLeaf.app"' "$tmp/Casks/fiveleaf.rb"
grep -Fxq '  command_wrapper "ff", content: <<~SH' "$tmp/Casks/fiveleaf.rb"
grep -Fxq '  homepage "https://github.com/example/FiveLeaf"' "$tmp/Casks/fiveleaf.rb"
grep -Fxq '  url "https://github.com/example/FiveLeaf/releases/download/v#{version}/fiveleaf-#{version}-#{arch}.dmg"' "$tmp/Casks/fiveleaf.rb"
if grep -Eq '@[A-Z_]+@|sorrycc/|mayoi' "$tmp/Casks/fiveleaf.rb"; then
  echo "generated cask contains an upstream address or unfilled template" >&2
  exit 1
fi
cp "$tmp/Casks/fiveleaf.rb" "$tmp/original.rb"

# A release updates its version and checksums and keeps unrelated edits.
sed 's/command_wrapper "ff"/command_wrapper "fiveleaf"/; s/echo "ff:/echo "fiveleaf:/; s/Small, fast code editor/Edited description/; /  app "FiveLeaf.app"/a\
  zap trash: "~/Library/FiveLeaf"\
' "$tmp/original.rb" > "$tmp/Casks/fiveleaf.rb"
sh "$tmp/scripts/update-homebrew.sh" 0.12.9
grep -Fxq '  version "0.12.9"' "$tmp/Casks/fiveleaf.rb"
for arch in arm64 amd64; do
  sum="$(printf 'fiveleaf-0.12.9-%s.dmg\n' "$arch" | shasum -a 256 | cut -d ' ' -f 1)"
  grep -Fq "\"$sum\"" "$tmp/Casks/fiveleaf.rb"
done
grep -Fq '  desc "Edited description"' "$tmp/Casks/fiveleaf.rb"
grep -Fq '  zap trash: "~/Library/FiveLeaf"' "$tmp/Casks/fiveleaf.rb"
grep -Fxq '  command_wrapper "ff", content: <<~SH' "$tmp/Casks/fiveleaf.rb"
grep -Fq 'echo "ff: FiveLeaf.app is not found"' "$tmp/Casks/fiveleaf.rb"
grep -Fxq '    FIVELEAF_CLI=1 exec "$app" "$@"' "$tmp/Casks/fiveleaf.rb"
cp "$tmp/Casks/fiveleaf.rb" "$tmp/updated.rb"
sh "$tmp/scripts/update-homebrew.sh" 0.12.9
cmp "$tmp/updated.rb" "$tmp/Casks/fiveleaf.rb"

# Missing or duplicate fields must fail before replacing the cask.
for layout in missing duplicate; do
  case "$layout" in
    missing) sed '/^  sha256 /d' "$tmp/original.rb" ;;
    duplicate) cat "$tmp/original.rb"; printf '  version "0.12.2"\n' ;;
  esac > "$tmp/Casks/fiveleaf.rb"
  cp "$tmp/Casks/fiveleaf.rb" "$tmp/before.rb"
  if sh "$tmp/scripts/update-homebrew.sh" 0.12.9 > "$tmp/output" 2>&1; then
    echo "accepted the $layout cask layout" >&2
    exit 1
  fi
  grep -Fq 'unexpected cask layout' "$tmp/output"
  cmp "$tmp/before.rb" "$tmp/Casks/fiveleaf.rb"
done

# No repository, or text that cannot be an owner/repository, leaves the cask alone.
for repo in '' 'invalid' 'owner/repo/extra' 'owner/repo"'; do
  cp "$tmp/Casks/fiveleaf.rb" "$tmp/before.rb"
  if FIVELEAF_REPOSITORY="$repo" GITHUB_REPOSITORY='' sh "$tmp/scripts/update-homebrew.sh" 0.12.9 > "$tmp/output" 2>&1; then
    echo "accepted an invalid release repository" >&2
    exit 1
  fi
  grep -Fq 'set FIVELEAF_REPOSITORY or GITHUB_REPOSITORY' "$tmp/output"
  cmp "$tmp/before.rb" "$tmp/Casks/fiveleaf.rb"
done

# GitHub Actions supplies the current fork through GITHUB_REPOSITORY.
cp "$tmp/original.rb" "$tmp/Casks/fiveleaf.rb"
FIVELEAF_REPOSITORY='' GITHUB_REPOSITORY=example/FiveLeaf sh "$tmp/scripts/update-homebrew.sh" 0.12.9
grep -Fxq '  version "0.12.9"' "$tmp/Casks/fiveleaf.rb"
echo "Homebrew release update tests passed"
