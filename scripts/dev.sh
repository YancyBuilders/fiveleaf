#!/usr/bin/env bash
# Runs FiveLeaf Dev with live reload (go tool mygo dev) beside the everyday
# FiveLeaf. MyGo gives it the identifier dev.fiveleaf.editor.dev, so its own
# instance lock; FIVELEAF_DATA_DIR gives it its own settings and window state.
# Usage: scripts/dev.sh
# FIVELEAF_DATA_DIR picks another data folder (default: FiveLeaf-dev beside FiveLeaf's).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export FIVELEAF_DATA_DIR="${FIVELEAF_DATA_DIR:-$HOME/Library/Application Support/FiveLeaf-dev}"

# MyGo creates its own data folder, but not one set with SetPath.
mkdir -p "$FIVELEAF_DATA_DIR"

echo "==> mygo dev (data in $FIVELEAF_DATA_DIR)"
cd "$ROOT"
exec go tool mygo dev
