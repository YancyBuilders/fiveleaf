#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
version=0.11.1
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cd "$work"
npm pack --ignore-scripts --silent "elkjs@$version" >/dev/null
tar -xzf "elkjs-$version.tgz" package/LICENSE.md package/lib/elk.bundled.js
{
  printf '/*! elkjs %s, https://github.com/kieler/elkjs (EPL-2.0)\n' "$version"
  sed 's/^/ * /' package/LICENSE.md
  printf ' */\n'
  cat package/lib/elk.bundled.js
} | gzip -9n >"$root/cmd/fiveleaf/elk.js.gz"
