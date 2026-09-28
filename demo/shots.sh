#!/usr/bin/env bash
# Takes the screenshots for shrippen.github.io/demo/tools/screenshots.py (kind "command").
# Reads SHOT_PLAN (the shots as JSON), SHOT_DIR, DEMO_LANG, DEMO_THEME, DEMO_TODAY.
set -euo pipefail
source "$(dirname "$0")/common.sh"
LANG_="${DEMO_LANG:-de}"
DIR="$(demo_dir "${LANG_}")"

DEMO_THEME="${DEMO_THEME:-kante}" "${BUILD}/hansei" demo --lang "${LANG_}" --dir "${DIR}" prepare
"${BUILD}/hansei" demo --lang "${LANG_}" --dir "${DIR}" daemon &
DAEMON=$!
trap 'kill ${DAEMON} 2>/dev/null' EXIT
sleep 1

# Offscreen window; the runner (demo builds only) walks SHOT_PLAN, saves <name>.png, quits.
HANSEI_SOCKET="${DIR}/hansei.sock" LANGUAGE="${LANG_}" QT_QPA_PLATFORM=offscreen \
    "${BUILD}/kde/bin/hansei-kde" -geometry 1600x950
