#!/usr/bin/env bash
# Copies the Kante design system into Hansei.
#
#   kde/qml/Kante            KanteStyle, wrappers, skins, fonts (the KDE app, via qrc)
#   tui/kante/palette.json   the token source for the terminal styles
#
# Source: KANTE_DS, default ../shrippen.github.io/kante.
# The copies are not edited here; change the design system and sync again.
set -euo pipefail
cd "$(dirname "$0")/.."

DS="${KANTE_DS:-../shrippen.github.io/kante}"
if [ ! -f "$DS/qml/Kante/qmldir" ] || [ ! -f "$DS/tokens/palette.json" ]; then
    echo "sync-kante: no Kante design system in $DS (set KANTE_DS)" >&2
    exit 1
fi

rm -rf kde/qml/Kante
mkdir -p kde/qml tui/kante
cp -r "$DS/qml/Kante" kde/qml/Kante
cp "$DS/tokens/palette.json" tui/kante/palette.json

echo "sync-kante: $(git -C "$DS" describe --always --dirty 2>/dev/null || echo unknown) -> kde/qml/Kante, tui/kante/palette.json"
