#!/usr/bin/env bash
set -euo pipefail

unformatted="$(gofmt -l cmd src)"
if [[ -n "$unformatted" ]]; then
  printf 'Go files need formatting:\n%s\n' "$unformatted"
  exit 1
fi

go vet ./...
npm ci
bash scripts/tests.sh
