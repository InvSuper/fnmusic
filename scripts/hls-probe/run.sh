#!/usr/bin/env bash
set -euo pipefail

module=$(cd -- "$(dirname -- "$0")" && pwd)
target=${1:-linux}
case "$target" in
  linux)
    CGO_ENABLED=0 go -C "$module" run .
    ;;
  windows)
    parent=${2:?"pass a writable directory under /mnt/c"}
    out=$(mktemp -d "$parent/waygu-hls.XXXXXX")
    cleanup() {
      if [[ -f "$out/hls.exe" ]]; then unlink "$out/hls.exe"; fi
      rmdir "$out"
    }
    trap cleanup EXIT
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go -C "$module" build -o "$out/hls.exe" .
    cd /mnt/c/Windows
    timeout 30s "$out/hls.exe"
    ;;
  *) printf 'Unsupported target %s\n' "$target" >&2; exit 2 ;;
esac
