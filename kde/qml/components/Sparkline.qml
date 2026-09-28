import QtQuick
import QtQuick.Controls as QQC2
import org.kde.kirigami as Kirigami
import Kante

/** Conformity over time: one bar per sample on a base line, the newest highlighted; hover shows date and value. */
Item {
    id: spark

    property var values: []
    property var days: []
    readonly property real bar: Kirigami.Units.smallSpacing * 2.2
    readonly property real gap: 3

    implicitWidth: values.length * (bar + gap)
    implicitHeight: Kirigami.Units.gridUnit * 1.6

    Rectangle {
        width: parent.width
        height: 1
        y: parent.height
        color: KanteStyle.ruleColor
    }

    Repeater {
        model: spark.values
        Rectangle {
            id: bar
            required property real modelData
            required property int index
            x: index * (spark.bar + spark.gap)
            width: spark.bar
            height: Math.max(2, spark.height * modelData)
            y: spark.height - height
            color: index === spark.values.length - 1 ? KanteStyle.accentColor : Qt.alpha(KanteStyle.positiveTextColor, hover.hovered ? 1 : 0.6)

            HoverHandler { id: hover }
            QQC2.ToolTip.visible: hover.hovered
            QQC2.ToolTip.text: (spark.days[index] ? Qt.formatDate(new Date(spark.days[index] + "T12:00:00"), Qt.DefaultLocaleShortDate) + ": " : "") + Math.round(modelData * 100) + " %"
        }
    }
}
