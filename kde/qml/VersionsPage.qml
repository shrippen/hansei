import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante
import "components"

/**
 * One change over its rounds of feedback: every version stays, the diff shows what changed
 * from one round to the next, and you can go back to any of them.
 */
Kirigami.Page {
    id: page

    readonly property var store: applicationWindow().store
    property string batchID
    property string path
    property int hunkIndex: 0
    property var file: null
    property var batch: null
    property var compare: null
    property int from: 0
    property int to: 1
    property bool wholeFile: false

    readonly property string hunkID: file && file.hunks[hunkIndex] ? file.hunks[hunkIndex].id : ""
    // Only the rounds about this change: your feedback on it and the AI answers that made a new version of the file.
    readonly property var rounds: batch ? (batch.thread || []).filter(m => (m.hunk && m.hunk === hunkID)
        || (m.role !== "user" && m.versions && m.versions[path] !== undefined)) : []
    readonly property var versions: file ? [{ n: 0, author: "vault", created: "" }].concat(file.versions) : []

    title: i18n("History of change %1 · %2", hunkIndex + 1, store.fileName(path))
    padding: 0
    KantePageTitle { page: page }
    // Kante: the page ground is Kante's ground, not the dialog tint KanteScope hands to the theme.
    background: Rectangle { color: KanteStyle.themed ? KanteStyle.backgroundColor : Kirigami.Theme.backgroundColor }

    function load() {
        store.call("file", { id: batchID, path: path, mode: "split", context: 3 }, r => {
            if (!r) {
                return
            }
            file = r
            to = r.current
            from = Math.max(0, to - 1)
            loadCompare()
        })
        store.call("batch", { id: batchID }, r => { batch = r })
    }

    function loadCompare() {
        store.call("compare", { id: batchID, path: path, from: from, to: to, mode: "split", context: wholeFile ? -1 : 3 }, r => { compare = r })
    }

    function authorName(a) {
        return a === "ai" ? i18n("AI") : a === "user" ? i18n("you") : a === "rules" ? i18n("checks") : a === "vault" ? i18n("vault") : a
    }

    Component.onCompleted: load()

    QQC2.SplitView {
        anchors.fill: parent
        orientation: Qt.Horizontal

        ColumnLayout {
            QQC2.SplitView.preferredWidth: Kirigami.Units.gridUnit * 20
            QQC2.SplitView.minimumWidth: Kirigami.Units.gridUnit * 14
            spacing: Kirigami.Units.smallSpacing

            KanteSectionLabel { text: i18n("Versions"); Layout.margins: Kirigami.Units.largeSpacing; Layout.bottomMargin: 0 }
            // Click a version to see what it changed against the one before.
            Repeater {
                model: page.versions
                delegate: QQC2.ItemDelegate {
                    id: ver
                    required property var modelData
                    Layout.fillWidth: true
                    Layout.leftMargin: Kirigami.Units.smallSpacing
                    Layout.rightMargin: Kirigami.Units.smallSpacing
                    highlighted: page.to === modelData.n
                    onClicked: {
                        page.to = modelData.n
                        page.from = Math.max(0, modelData.n - 1)
                        page.loadCompare()
                    }
                    contentItem: RowLayout {
                        QQC2.Label {
                            text: ver.modelData.n === 0 ? i18n("Vault") : "v" + ver.modelData.n
                            font: KanteStyle.monoFont(Kirigami.Theme.defaultFont.pointSize, true)
                            Layout.preferredWidth: Kirigami.Units.gridUnit * 3
                        }
                        QQC2.Label {
                            text: ver.modelData.n === 0 ? i18n("before the batch") : page.authorName(ver.modelData.author) + (ver.modelData.created ? " · " + Qt.formatTime(new Date(ver.modelData.created), Qt.DefaultLocaleShortDate) : "")
                            color: ver.highlighted ? Kirigami.Theme.highlightedTextColor : KanteStyle.mutedTextColor
                            elide: Text.ElideRight
                            Layout.fillWidth: true
                        }
                        KanteChip {
                            interactive: false
                            visible: page.file && ver.modelData.n === page.file.current
                            text: i18n("in use")
                            chipColor: ver.highlighted ? KanteStyle.accentForegroundColor : KanteStyle.positiveTextColor
                        }
                        KanteToolButton {
                            visible: page.file && ver.modelData.n > 0 && ver.modelData.n !== page.file.current
                            text: i18n("Use")
                            icon.name: "checkmark"
                            onClicked: page.store.call("setVersion", { id: page.batchID, path: page.path, n: ver.modelData.n }, () => page.load())
                            QQC2.ToolTip.visible: hovered
                            QQC2.ToolTip.text: i18n("Use this version")
                        }
                    }
                }
            }

            KanteSectionLabel { text: i18n("Rounds about this change"); Layout.margins: Kirigami.Units.largeSpacing; Layout.bottomMargin: 0 }
            ListView {
                Layout.fillWidth: true
                Layout.fillHeight: true
                Layout.leftMargin: Kirigami.Units.largeSpacing
                Layout.rightMargin: Kirigami.Units.largeSpacing
                clip: true
                spacing: Kirigami.Units.smallSpacing
                model: page.rounds
                delegate: QQC2.Control {
                    required property var modelData
                    width: ListView.view.width
                    padding: Kirigami.Units.largeSpacing
                    background: Surface { fill: modelData.role === "user" ? KanteStyle.cardColor : KanteStyle.sunkenColor }
                    contentItem: ColumnLayout {
                        RowLayout {
                            KanteSectionLabel { text: modelData.role === "user" ? i18n("You") : modelData.role === "system" ? i18n("Round") : i18n("AI") }
                            KanteChip {
                                interactive: false
                                visible: modelData.versions && modelData.versions[page.path] !== undefined
                                text: modelData.versions ? i18n("→ v%1", modelData.versions[page.path]) : ""
                                chipColor: KanteStyle.accentTextColor
                            }
                        }
                        QQC2.Label { text: modelData.text; wrapMode: Text.Wrap; Layout.fillWidth: true }
                    }
                }
                // Empty: say so right under the heading, not in the middle of the column.
                QQC2.Label {
                    visible: page.rounds.length === 0
                    width: parent.width
                    text: i18n("No feedback on this change yet")
                    color: KanteStyle.mutedTextColor
                    wrapMode: Text.Wrap
                }
            }
        }

        ColumnLayout {
            QQC2.SplitView.fillWidth: true
            spacing: 0

            RowLayout {
                Layout.margins: Kirigami.Units.largeSpacing
                KanteSectionLabel {
                    text: page.from === 0 ? i18n("Vault → v%1", page.to) : i18n("v%1 → v%2", page.from, page.to)
                    Layout.fillWidth: true
                }
                KanteToolButton {
                    text: i18n("Whole file")
                    icon.name: "document-preview"
                    checkable: true
                    checked: page.wholeFile
                    onToggled: { page.wholeFile = checked; page.loadCompare() }
                }
            }
            DiffView {
                id: cmp
                visible: !page.compare || page.compare.rows.length > 0
                Layout.fillWidth: true
                Layout.fillHeight: true
                headers: false
                file: page.file ? Object.assign({}, page.file, { rows: page.compare ? page.compare.rows : [] }) : null
                onShowAll: { page.wholeFile = true; page.loadCompare() }
            }
            Kirigami.PlaceholderMessage {
                visible: page.compare && page.compare.rows.length === 0
                Layout.fillWidth: true
                Layout.fillHeight: true
                text: i18n("No difference")
            }
        }
    }
}
