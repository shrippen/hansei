#!/usr/bin/env bash
# Extracts the translatable strings into po/hansei.pot (KDE convention).
# Update a translation afterwards: msgmerge -U po/de/hansei.po po/hansei.pot
set -euo pipefail
cd "$(dirname "$0")"
find qml -path qml/Kante -prune -o -name '*.qml' -print | sort > .qmlfiles
xgettext --from-code=UTF-8 -L JavaScript -ki18n:1 -ki18nc:1c,2 -ki18np:1,2 -ki18ncp:1c,2,3 -o .qml.pot -f .qmlfiles
xgettext --from-code=UTF-8 -C -ki18n:1 -ki18nc:1c,2 -ki18np:1,2 -o .cpp.pot src/*.cpp
msgcat .qml.pot .cpp.pot -o po/hansei.pot
rm -f .qmlfiles .qml.pot .cpp.pot
echo "po/hansei.pot: $(grep -c '^msgid' po/hansei.pot) strings"
