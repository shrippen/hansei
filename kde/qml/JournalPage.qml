import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante
import "components"

/**
 * Every write to the vault, newest first and grouped by batch. Open an entry to see
 * exactly what it changed before you undo it.
 */
Kirigami.ScrollablePage {
    id: page

    readonly property var store: applicationWindow().store
    property var entries: []
    property string search: ""
    property string show: "all"      // all, written, undone
    property string expanded: ""     // journal entry ID with its diff open
    property var diffRows: null

    readonly property var groups: {
        const out = []
        const byBatch = {}
        const q = search.toLowerCase()
        for (const e of entries) {
            if (show === "written" && e.undone || show === "undone" && !e.undone) {
                continue
            }
            if (q && e.path.toLowerCase().indexOf(q) < 0 && (e.title || "").toLowerCase().indexOf(q) < 0) {
                continue
            }
            let g = byBatch[e.batch]
            if (!g) {
                g = { batch: e.batch, title: e.title, time: e.time, day: dayOf(e.time), entries: [] }
                byBatch[e.batch] = g
                out.push(g)
            }
            g.entries.push(e)
        }
        return out
    }

    title: i18n("Journal")
    KantePageTitle { page: page }
    // Kante: the page ground is Kante's ground, not the dialog tint KanteScope hands to the theme.
    background: Rectangle { color: KanteStyle.themed ? KanteStyle.backgroundColor : Kirigami.Theme.backgroundColor }

    // "Today", "Yesterday" or the date: the heading above a day's batches.
    function dayOf(time) {
        const d = new Date(time)
        const today = new Date()
        today.setHours(0, 0, 0, 0)
        const diff = Math.round((new Date(d.getFullYear(), d.getMonth(), d.getDate()) - today) / 86400000)
        return diff === 0 ? i18n("Today") : diff === -1 ? i18n("Yesterday") : Qt.formatDate(d, Qt.DefaultLocaleLongDate)
    }

    function load() {
        store.call("journal", { limit: 500 }, r => { entries = r || [] })
    }

    function toggle(id) {
        if (expanded === id) {
            expanded = ""
            return
        }
        expanded = id
        diffRows = null
        store.call("journalDiff", { id: id }, r => { if (r && expanded === id) diffRows = r.rows })
    }

    Component.onCompleted: load()
    Connections {
        target: page.store
        function onBatchChanged() { page.load() }
    }

    header: QQC2.ToolBar {
        contentItem: RowLayout {
            Kirigami.SearchField {
                KanteFieldSkin { control: parent }
                Layout.fillWidth: true
                Layout.maximumWidth: Kirigami.Units.gridUnit * 20
                placeholderText: i18n("Search notes and batches…")
                onTextChanged: page.search = text
                onActiveFocusChanged: page.store.typing = activeFocus
            }
            Item { Layout.fillWidth: true }
            QQC2.ButtonGroup { id: filters }
            Repeater {
                model: [["all", i18nc("journal filter", "All")], ["written", i18nc("journal filter", "Written")], ["undone", i18nc("journal filter", "Reverted")]]
                delegate: KanteToolButton {
                    required property var modelData
                    text: modelData[1]
                    checkable: true
                    checked: page.show === modelData[0]
                    QQC2.ButtonGroup.group: filters
                    onClicked: page.show = modelData[0]
                }
            }
        }
    }

    ColumnLayout {
        spacing: Kirigami.Units.largeSpacing

        Kirigami.PlaceholderMessage {
            visible: page.groups.length === 0
            Layout.fillWidth: true
            Layout.topMargin: Kirigami.Units.gridUnit * 4
            icon.name: "view-history"
            text: page.entries.length === 0 ? i18n("Nothing written yet") : i18n("Nothing matches")
            explanation: page.entries.length === 0 ? i18n("Accepted changes appear here and can be undone.") : ""
        }

        Repeater {
            model: page.groups
            delegate: ColumnLayout {
                id: group
                required property var modelData
                required property int index
                readonly property bool anyWritten: modelData.entries.some(e => !e.undone)
                Layout.fillWidth: true
                spacing: 0

                // A day heading when the day changes.
                Kirigami.Heading {
                    visible: group.index === 0 || page.groups[group.index - 1].day !== group.modelData.day
                    text: group.modelData.day
                    level: 3
                    font: KanteStyle.headingFont(Kirigami.Theme.defaultFont.pointSize * 1.15)
                    Layout.topMargin: group.index === 0 ? 0 : Kirigami.Units.gridUnit
                    Layout.bottomMargin: Kirigami.Units.smallSpacing
                }
                RowLayout {
                    Layout.fillWidth: true
                    SectionLabel {
                        text: group.modelData.title || i18n("Batch")
                        Layout.fillWidth: true
                    }
                    QQC2.Label {
                        text: i18np("one file", "%1 files", group.modelData.entries.length)
                        font: Kirigami.Theme.smallFont
                        color: KanteStyle.mutedTextColor
                    }
                    KanteToolButton {
                        visible: group.anyWritten && group.modelData.entries.length > 1
                        text: i18n("Undo batch")
                        icon.name: "edit-undo"
                        onClicked: page.store.call("undoBatch", { id: group.modelData.batch }, () => page.load())
                    }
                }

                Repeater {
                    model: group.modelData.entries
                    delegate: ColumnLayout {
                        id: entry
                        required property var modelData
                        readonly property bool open: page.expanded === modelData.id
                        Layout.fillWidth: true
                        spacing: 0

                        QQC2.ItemDelegate {
                            Layout.fillWidth: true
                            highlighted: entry.open
                            onClicked: page.toggle(entry.modelData.id)
                            contentItem: RowLayout {
                                spacing: Kirigami.Units.largeSpacing
                                Kirigami.Icon {
                                    source: entry.open ? "go-down" : "go-next"
                                    implicitWidth: Kirigami.Units.iconSizes.small
                                    implicitHeight: Kirigami.Units.iconSizes.small
                                }
                                QQC2.Label {
                                    text: Qt.formatTime(new Date(entry.modelData.time), Qt.DefaultLocaleShortDate)
                                    font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                                    color: KanteStyle.mutedTextColor
                                }
                                QQC2.Label {
                                    text: entry.modelData.path
                                    font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                                    color: KanteStyle.textColor
                                    elide: Text.ElideMiddle
                                    Layout.fillWidth: true
                                }
                                QQC2.Label { text: "+" + entry.modelData.added; color: KanteStyle.positiveTextColor; font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize) }
                                QQC2.Label { text: "−" + entry.modelData.removed; color: KanteStyle.negativeTextColor; font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize) }
                                Chip { visible: entry.modelData.new; text: i18n("new"); tone: KanteStyle.infoColor }
                                Chip { visible: entry.modelData.undone; text: i18n("reverted"); tone: KanteStyle.mutedTextColor }
                                KanteButton {
                                    visible: !entry.modelData.undone
                                    text: i18n("Undo")
                                    icon.name: "edit-undo"
                                    onClicked: page.store.call("undoFile", { id: entry.modelData.batch, path: entry.modelData.path }, () => page.load())
                                }
                            }
                        }

                        // What this write changed, before you undo it.
                        ColumnLayout {
                            visible: entry.open
                            Layout.fillWidth: true
                            Layout.leftMargin: Kirigami.Units.gridUnit * 1.5
                            Layout.bottomMargin: Kirigami.Units.largeSpacing
                            QQC2.BusyIndicator { visible: entry.open && page.diffRows === null; running: visible }
                            DiffView {
                                visible: entry.open && page.diffRows !== null
                                Layout.fillWidth: true
                                Layout.preferredHeight: Math.min(Kirigami.Units.gridUnit * 18, Math.max(Kirigami.Units.gridUnit * 3, (page.diffRows || []).length * Kirigami.Units.gridUnit * 1.1))
                                headers: false
                                file: entry.open && page.diffRows ? { mode: "split", rows: page.diffRows, hunks: [], status: "applied" } : null
                            }
                            RowLayout {
                                visible: entry.open
                                KanteButton {
                                    text: i18n("Show in the review")
                                    icon.name: "document-compare"
                                    onClicked: {
                                        page.store.batchID = entry.modelData.batch
                                        page.store.path = entry.modelData.path
                                        applicationWindow().show("review")
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
