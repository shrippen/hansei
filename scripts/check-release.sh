#!/usr/bin/env bash
# Release gate: builds what ships and fails if demo code or data got in.
#   scripts/check-release.sh [version]
# The demo exists only with `go build -tags demo` and `-DHANSEI_DEMO=ON`; releases use neither.
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION="${1:-dev}"
OUT="$(mktemp -d)"
trap 'rm -rf "${OUT}"' EXIT

go test ./...
go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o "${OUT}/hansei" ./cmd/hansei

# Go: no demo command, no demo world, no scripted AI.
for marker in "hansei-demo" "Studio Weber" "studio-ki" "it_docs" "demo --lang"; do
    if strings "${OUT}/hansei" | grep -q -- "${marker}"; then
        echo "check-release: demo marker '${marker}' found in the Go binary" >&2
        exit 1
    fi
done
if "${OUT}/hansei" demo prepare >/dev/null 2>&1; then
    echo "check-release: the release binary knows the demo command" >&2
    exit 1
fi

# KDE app: default options, no screenshot runner.
cmake -S kde -B "${OUT}/kde" -DCMAKE_BUILD_TYPE=Release -G Ninja >/dev/null
cmake --build "${OUT}/kde" >/dev/null
for marker in "ScreenshotRunner" "SHOT_PLAN" "ShotPlan"; do
    if strings "${OUT}/kde/bin/hansei-kde" | grep -q -- "${marker}"; then
        echo "check-release: demo marker '${marker}' found in hansei-kde" >&2
        exit 1
    fi
done
echo "check-release: ok (${VERSION})"
