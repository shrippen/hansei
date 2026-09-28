# Shared by demo/start.sh and demo/shots.sh: builds the demo binaries (internal, never released).
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD="${ROOT}/build-demo"
mkdir -p "${BUILD}"

# Go: the demo command only exists with -tags demo.
(cd "${ROOT}" && go build -tags demo -o "${BUILD}/hansei" ./cmd/hansei)

# KDE app with the screenshot runner (HANSEI_DEMO=ON).
if [ -z "${HANSEI_TUI_ONLY:-}" ]; then
    cmake -S "${ROOT}/kde" -B "${BUILD}/kde" -DHANSEI_DEMO=ON -DCMAKE_BUILD_TYPE=Release -G Ninja >/dev/null
    cmake --build "${BUILD}/kde" >/dev/null
fi

# The demo folder name must contain "hansei-demo"; `hansei demo prepare` refuses anything else.
demo_dir() { echo "${XDG_RUNTIME_DIR:-/tmp}/hansei-demo-$1"; }
