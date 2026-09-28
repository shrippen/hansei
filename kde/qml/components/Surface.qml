import QtQuick
import org.kde.kirigami as Kirigami
import Kante

/**
 * A card surface. Kante and Kante Light: the cut corner, with the accent bar on top
 * only for active cards (bar). System: a plain framed card like Kirigami's; the bar
 * is not drawn there, selection shows in the frame.
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
    }
}
