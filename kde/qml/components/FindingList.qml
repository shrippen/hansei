import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante

/**
 * The findings of one check, one row per finding: note, line, masked excerpt and a
 * link to open the note in Obsidian. Used on the start page and on workbench cards.
 */
ColumnLayout {
    id: list

    required property var store
    property string rule
    property int limit: 12
    // Found notes of a find-only task, {path, reason}; used instead of rule findings.
    property var found: null

    readonly property var items: found !== null ? found.map(f => ({ path: f.path, line: 0, excerpt: f.reason, detail: "" })) : store.findingsOf(rule)
    spacing: 0

    Component.onCompleted: {
        if (found === null && (store.findings.findings || []).length === 0) {
            store.refreshFindings()
        }
    }

    Repeater {
        model: list.items.slice(0, list.limit)
        delegate: QQC2.ItemDelegate {
            id: row
            required property var modelData
            Layout.fillWidth: true
            padding: Kirigami.Units.smallSpacing
            onClicked: Qt.openUrlExternally(list.store.obsidianUrl(modelData.path))
            QQC2.ToolTip.visible: hovered
            QQC2.ToolTip.delay: Kirigami.Units.toolTipDelay
            QQC2.ToolTip.text: i18n("Open %1 in Obsidian", modelData.path)
            contentItem: RowLayout {
                spacing: Kirigami.Units.largeSpacing
                QQC2.Label {
                    text: list.store.fileName(row.modelData.path) + (row.modelData.line > 0 ? ":" + row.modelData.line : "")
                    font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                    Layout.preferredWidth: Kirigami.Units.gridUnit * 12
                    Layout.maximumWidth: list.width * 0.4
                    elide: Text.ElideMiddle
                }
                QQC2.Label {
                    text: [row.modelData.detail, row.modelData.excerpt].filter(s => !!s).join(" · ")
                    color: KanteStyle.mutedTextColor
                    font: Kirigami.Theme.smallFont
                    elide: Text.ElideRight
                    Layout.fillWidth: true
                }
                Kirigami.Icon {
                    source: "document-open"
                    implicitWidth: Kirigami.Units.iconSizes.small
                    implicitHeight: Kirigami.Units.iconSizes.small
                    opacity: row.hovered ? 1 : 0.4
                }
            }
        }
    }

    QQC2.Label {
        visible: list.items.length > list.limit
        text: i18np("and one more", "and %1 more", list.items.length - list.limit)
        color: KanteStyle.mutedTextColor
        font: Kirigami.Theme.smallFont
        leftPadding: Kirigami.Units.smallSpacing
    }
}
