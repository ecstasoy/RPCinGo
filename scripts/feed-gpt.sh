#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   ./bundle_go.sh [root_dir] [output_file]
# Examples:
#   ./bundle_go.sh . go_bundle.txt
#   ./bundle_go.sh /path/to/repo /tmp/repo_go.txt

ROOT_DIR="${1:-.}"
OUT_FILE="${2:-go_bundle.txt}"

# Create/overwrite output
: > "$OUT_FILE"

# Optional: record context info at the top
{
  echo "=== GO SOURCE BUNDLE ==="
  echo "Root: $(cd "$ROOT_DIR" && pwd)"
  echo "Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  echo
} >> "$OUT_FILE"

# Find .go files, sort for stable ordering, and append them with headers
# - Excludes common junk dirs (vendor, .git). Add more if you want.
find "$ROOT_DIR" \
  -type d \( -name .git -o -name vendor -o -name node_modules -o -name dist -o -name build \) -prune -false \
  -o -type f -name '*.go' -print0 \
| sort -z \
| while IFS= read -r -d '' file; do
    rel="$file"
    # Try to print a nicer relative path if ROOT_DIR is a prefix
    if [[ "$file" == "$ROOT_DIR"* ]]; then
      rel="${file#"$ROOT_DIR"/}"
    fi

    {
      echo "----- FILE: $rel -----"
      echo "----- BEGIN -----"
      cat "$file"
      echo
      echo "----- END: $rel -----"
      echo
    } >> "$OUT_FILE"
  done

echo "Wrote combined output to: $OUT_FILE"
