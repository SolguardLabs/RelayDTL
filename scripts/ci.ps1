$ErrorActionPreference = 'Stop'

$unformatted = @(gofmt -l cmd src)
if ($LASTEXITCODE -ne 0 -or $unformatted.Count -gt 0) {
    throw "Go formatting check failed: $($unformatted -join ', ')"
}

& go test ./...
if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
& go vet ./...
if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
& npm.cmd ci
if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
& npm.cmd test
if ($LASTEXITCODE -ne 0) { throw 'Node tests failed' }
