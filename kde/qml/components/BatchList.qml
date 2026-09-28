import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante

/** Batches waiting for review, grouped by topic; the selected one lists its files. */
ListView {
    id: list

    property var batches: []
    property var batch: null          // BatchView of the selected batch
    property string selected: ""
    property string path: ""
    property var progress: ({})

    signal pickBatch(string id)
    signal pickFile(string path)

    clip: true
    spacing: Kirigami.Units.smallSpacing
    model: batches
    QQC2.ScrollBar.vertical: QQC2.ScrollBar {}

    header: SectionLabel {
        text: i18n("Batches by topic")
        padding: Kirigami.Units.smallSpacing
        bottomPadding: Kirigami.Units.largeSpacing
    }

    delegate: QQC2.ItemDelegate {
        id: item

        required property var modelData
        readonly property bool active: modelData.id === list.selected

        width: ListView.view.width
        padding: Kirigami.Units.largeSpacing
        highlighted: false
        onClicked: list.pickBatch(modelData.id)

        background: Surface {
            selected: item.active
            fill: item.active ? (KanteStyle.active ? KanteStyle.cardColor : Kirigami.Theme.backgroundColor) : "transparent"
            bar: item.active ? KanteStyle.accentColor : "transparent"
        }

        contentItem: ColumnLayout {
            spacing: Kirigami.Units.smallSpacing

            Chip {
                text: item.modelData.topic || i18n("Batch")
                tone: KanteStyle.tagColor
            }
            QQC2.Label {
                text: item.modelData.title
                wrapMode: Text.Wrap
                font.bold: item.active
                Layout.fillWidth: true
            }
            ProgressStrip {
                counts: item.modelData.counts
                Layout.fillWidth: true
            }
            // Figures in mono, on their own line so the topic chip never cuts them.
            QQC2.Label {
                readonly property var c: item.modelData.counts
                text: i18n("Files %1/%2 · Changes %3/%4", c.done, c.files, c.accepted + c.rejected, c.hunks)
                font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize * 0.85)
                color: KanteStyle.mutedTextColor
                elide: Text.ElideRight
                Layout.fillWidth: true
            }
            QQC2.Label {
                visible: item.modelData.running || item.modelData.revising
                text: i18n("AI: %1", list.progress[item.modelData.id] || i18n("working…"))
                color: KanteStyle.neutralTextColor
                font: Kirigami.Theme.smallFont
                elide: Text.ElideRight
                Layout.fillWidth: true
            }
            QQC2.Label {
                visible: item.modelData.questions > 0 && !item.modelData.running
                text: i18np("One question from the AI", "%1 questions from the AI", item.modelData.questions)
                color: KanteStyle.infoColor
                font: Kirigami.Theme.smallFont
            }

            Repeater {
                model: item.active && list.batch ? list.batch.files : []
                delegate: QQC2.ItemDelegate {
                    id: fileItem
                    required property var modelData
                    Layout.fillWidth: true
                    padding: Kirigami.Units.smallSpacing
                    readonly property bool current: modelData.path === list.path
                    onClicked: list.pickFile(modelData.path)
                    background: Rectangle {
                        color: fileItem.current ? Qt.alpha(KanteStyle.accentColor, 0.18) : (fileItem.hovered ? KanteStyle.sunkenColor : "transparent")
                        Rectangle {
                            visible: fileItem.current
                            width: 2
                            height: parent.height
                            color: KanteStyle.accentColor
                        }
                    }
                    readonly property string stateText: modelData.status === "applied" ? i18n("Written to the vault")
                        : modelData.status === "stale" ? i18n("The note changed since the proposal")
                        : modelData.status === "skipped" ? i18n("Everything rejected, nothing written")
                        : modelData.status === "undone" ? i18n("Reverted")
                        : modelData.feedback ? i18n("New version after your feedback") : i18n("Open")
                    QQC2.ToolTip.visible: hovered
                    QQC2.ToolTip.delay: Kirigami.Units.toolTipDelay
                    QQC2.ToolTip.text: modelData.path + "\n" + stateText
                    contentItem: RowLayout {
                        Kirigami.Icon {
                            source: fileItem.modelData.status === "applied" ? "emblem-checked"
                                : fileItem.modelData.status === "stale" ? "emblem-warning"
                                : (fileItem.modelData.status === "skipped" || fileItem.modelData.status === "undone") ? "edit-undo"
                                : fileItem.modelData.feedback ? "dialog-messages" : "document-edit"
                            color: fileItem.modelData.status === "applied" ? KanteStyle.positiveTextColor
                                : fileItem.modelData.status === "stale" ? KanteStyle.neutralTextColor : Kirigami.Theme.textColor
                            isMask: true
                            Layout.preferredWidth: Kirigami.Units.iconSizes.small
                            Layout.preferredHeight: Kirigami.Units.iconSizes.small
                        }
                        QQC2.Label {
                            text: fileItem.modelData.path.split("/").pop()
                            font.bold: fileItem.current
                            elide: Text.ElideMiddle
                            Layout.fillWidth: true
                        }
                        QQC2.Label {
                            visible: fileItem.modelData.open > 0
                            text: i18np("one open", "%1 open", fileItem.modelData.open)
                            font: Kirigami.Theme.smallFont
                            color: KanteStyle.mutedTextColor
                        }
                    }
                }
            }
        }
    }

    Kirigami.PlaceholderMessage {
        anchors.centerIn: parent
        width: parent.width - Kirigami.Units.gridUnit * 2
        visible: list.count === 0
        icon.name: "checkmark"
        text: i18n("Nothing to review")
        explanation: i18n("Start a task or create a batch from the findings on the start page.")
    }
}
