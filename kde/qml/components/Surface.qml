import QtQuick
import org.kde.kirigami as Kirigami
import Kante

/**
 * A card surface: Kante's cut corner in Kante and Kante Light, a plain rounded
 * card in the System style. bar paints the accent bar (Kante) or a left edge (System).
 */
Item {
    id: surface

    property color bar: "transparent"
    property bool raised: false
    property bool selected: false
    property color fill: KanteStyle.active ? KanteStyle.cardColor : (raised ? Kirigami.Theme.backgroundColor : Kirigami.Theme.alternateBackgroundColor)

    KanteCard {
        anchors.fill: parent
        visible: KanteStyle.active
        color: surface.fill
        barColor: surface.bar
        borderColor: surface.selected ? KanteStyle.accentColor : KanteStyle.frameColor
    }

    Rectangle {
        anchors.fill: parent
        visible: !KanteStyle.active
        radius: Kirigami.Units.cornerRadius
        color: surface.fill
        border.width: 1
        border.color: surface.selected ? Kirigami.Theme.highlightColor : Qt.alpha(Kirigami.Theme.textColor, 0.15)

        Rectangle {
            visible: surface.bar.a > 0
            width: 3
            radius: 1
            color: surface.bar
            anchors { left: parent.left; top: parent.top; bottom: parent.bottom; margins: 1 }
        }
    }
}
