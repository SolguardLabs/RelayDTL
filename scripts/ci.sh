#!/usr/bin/env bash
set -euo pipefail

gofmt -l cmd src | tee /tmp/relaydtl-gofmt.txt
test ! -s /tmp/relaydtl-gofmt.txt
go test ./...
go vet ./...
npm ci
npm test
