set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

# Maintainer-only release plumbing (mirror + curated release branches). Optional
# import: the public tree ships without release.just and this justfile still works.
import? 'release.just'

# List recipes.
default:
    @just --list

# Unit-test the pure store + render logic (SDK-independent; green today).
test *ARGS:
    go test -race ./internal/... {{ARGS}}

# Vet + gofmt the whole tree (gofmt needs no compile; vet runs on internal/).
lint:
    go vet ./internal/...
    @test -z "$(gofmt -l . | tee /dev/stderr)" || { echo "gofmt issues (run \`just fmt\`)"; exit 1; }

# Format all Go sources in place.
fmt:
    gofmt -w .

# Build the binary to bin/ (needs the protocol-v2 SDK in ../terva; gated today).
build:
    @mkdir -p bin
    go build -trimpath -o bin/terva-tasks .

# Run a dev terva with this extension from source (needs a v2 host).
run *ARGS:
    terva --ext . {{ARGS}}

# Tidy go.mod / go.sum.
tidy:
    go mod tidy
