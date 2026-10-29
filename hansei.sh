#!/usr/bin/env bash
# Startet Hansei.
#   ./hansei.sh          KDE-App
#   ./hansei.sh tui      Terminal-Oberfläche
#   ./hansei.sh key      API-Schlüssel für Claude speichern (einmalig)
#   ./hansei.sh build    nur bauen
# Baut beim ersten Start (und nach Änderungen am Code) automatisch.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="${REPO}/bin/hansei"
APP="${REPO}/kde/build/bin/hansei-kde"

build() {
    echo "Baue Hansei …"
    (cd "${REPO}" && go build -trimpath -o "${BIN}" ./cmd/hansei)
    if [ ! -f "${REPO}/kde/build/build.ninja" ]; then
        cmake -S "${REPO}/kde" -B "${REPO}/kde/build" -DCMAKE_BUILD_TYPE=Release -G Ninja >/dev/null
    fi
    cmake --build "${REPO}/kde/build" >/dev/null
}

# Neu bauen, wenn etwas fehlt oder der Code neuer ist als die Programme.
needs_build() {
    [ ! -x "${BIN}" ] || [ ! -x "${APP}" ] && return 0
    [ -n "$(find "${REPO}/core" "${REPO}/cmd" "${REPO}/tui" "${REPO}/kde/src" "${REPO}/kde/qml" -newer "${APP}" -type f -print -quit)" ]
}

case "${1:-app}" in
    build)
        build
        exit 0
        ;;
    key)
        needs_build && build
        exec "${BIN}" key set claude
        ;;
    tui)
        needs_build && build
        exec "${BIN}"
        ;;
    app)
        needs_build && build
        # Die App startet den Dienst selbst; HANSEI_BIN zeigt ihr das gebaute Programm.
        HANSEI_BIN="${BIN}" exec "${APP}" "${@:2}"
        ;;
    *)
        sed -n '2,7p' "$0"
        exit 1
        ;;
esac
