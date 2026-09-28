import QtQuick
import QtQuick.Controls as QQC2
import org.kde.kirigami as Kirigami
import Kante

/** Review progress of a batch: accepted, rejected, open hunks. */
Item {
    id: strip

    property var counts: ({ hunks: 0, accepted: 0, rejected: 0 })

    implicitHeight: Math.round(Kirigami.Units.smallSpacing * 1.5)

    HoverHandler { id: hover }
    QQC2.ToolTip.visible: hover.hovered
    QQC2.ToolTip.delay: Kirigami.Units.toolTipDelay
    QQC2.ToolTip.text: i18n("%1 accepted · %2 rejected · %3 open", counts.accepted || 0, counts.rejected || 0, (counts.hunks || 0) - (counts.accepted || 0) - (counts.rejected || 0))
    Rectangle {
        anchors.fill: parent
        color: KanteStyle.sunkenColor
        radius: KanteStyle.active ? 0 : height / 2
    }
    Row {
        anchors.fill: parent
        Rectangle {
            width: strip.counts.hunks ? strip.width * strip.counts.accepted / strip.counts.hunks : 0
            height: parent.height
            color: KanteStyle.positiveTextColor
        }
        Rectangle {
            width: strip.counts.hunks ? strip.width * strip.counts.rejected / strip.counts.hunks : 0
            height: parent.height
            color: KanteStyle.negativeTextColor
        }
    }
}
