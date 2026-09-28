import QtQuick
import QtQuick.Controls as QQC2
import org.kde.kirigami as Kirigami
import Kante

/**
 * Small label chip: topics, states, quick reasons. Checkable when used as a toggle.
 * System: a filled, borderless badge like Breeze's; Kante: square with a thin frame.
 */
QQC2.AbstractButton {
    id: chip

    property color tone: Kirigami.Theme.textColor
    property bool interactive: false
    // Normal writing instead of Kante's uppercase label font, for longer text like rule names.
    property bool plain: false

    checkable: interactive
    hoverEnabled: interactive
    focusPolicy: interactive ? Qt.StrongFocus : Qt.NoFocus
    implicitHeight: label.implicitHeight + Kirigami.Units.smallSpacing * 1.5
    implicitWidth: label.implicitWidth + Kirigami.Units.largeSpacing * 1.2
    // Never wider than the row it sits in (Flow, Layout); the text elides.
    width: parent ? Math.min(implicitWidth, parent.width) : implicitWidth

    background: Rectangle {
        radius: KanteStyle.active ? 0 : height / 2
        color: KanteStyle.active
            ? (chip.checked ? Qt.alpha(chip.tone, 0.22) : (chip.hovered ? Qt.alpha(chip.tone, 0.1) : "transparent"))
            : Qt.alpha(chip.tone, chip.checked ? 0.3 : (chip.hovered ? 0.2 : 0.12))
        border.width: KanteStyle.active || chip.visualFocus ? 1 : 0
        border.color: KanteStyle.active ? Qt.alpha(chip.tone, chip.checked ? 0.9 : 0.45) : Kirigami.Theme.focusColor
    }

    contentItem: QQC2.Label {
        id: label
        text: chip.text
        color: chip.tone
        horizontalAlignment: Text.AlignHCenter
        verticalAlignment: Text.AlignVCenter
        font: KanteStyle.active && !chip.plain ? KanteStyle.labelFont() : Kirigami.Theme.smallFont
        elide: Text.ElideRight
    }
}
