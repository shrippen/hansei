#!/usr/bin/env bash
# Starts Hansei with the Studio Weber IT docs (internal, for screenshots; never shipped).
#   demo/start.sh [de|en] [app|tui]
# Own vault, config, data and socket under $XDG_RUNTIME_DIR/hansei-demo-LANG; a scripted AI,
# no real server, account or key is touched. DEMO_TODAY=YYYY-MM-DD fixes today,
# DEMO_THEME=system|kante-light|kante picks the style (default System).
set -euo pipefail
LANG_="${1:-${DEMO_LANG:-de}}"
MODE="${2:-app}"
[ "${MODE}" = "tui" ] && export HANSEI_TUI_ONLY=1
source "$(dirname "$0")/common.sh"
DIR="$(demo_dir "${LANG_}")"

"${BUILD}/hansei" demo --lang "${LANG_}" --dir "${DIR}" prepare
if [ "${MODE}" = "tui" ]; then
    exec "${BUILD}/hansei" demo --lang "${LANG_}" --dir "${DIR}" tui
fi

"${BUILD}/hansei" demo --lang "${LANG_}" --dir "${DIR}" daemon &
DAEMON=$!
trap 'kill ${DAEMON} 2>/dev/null' EXIT
sleep 1
HANSEI_SOCKET="${DIR}/hansei.sock" HANSEI_BIN="${BUILD}/hansei" LANGUAGE="${LANG_}" "${BUILD}/kde/bin/hansei-kde"
