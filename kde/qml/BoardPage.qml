import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante
import "components"

/**
 * The workbench: the same batches as the review line, as a board. Columns are the life of a
 * batch, from the findings of the checks to “done”. Click a card to review it.
 */
Kirigami.Page {
    id: page

    readonly property var store: applicationWindow().store
    readonly property var columns: [
        { key: "findings", title: i18n("Findings"), tone: KanteStyle.neutralTextColor },
        { key: "working", title: i18n("AI working"), tone: KanteStyle.accentColor },
        { key: "review", title: i18n("To review"), tone: KanteStyle.tagColor },
        { key: "feedback", title: i18n("Feedback open"), tone: KanteStyle.infoColor },
        { key: "done", title: i18n("Done"), tone: KanteStyle.positiveTextColor }
    ]

    title: i18n("Workbench")
    padding: Kirigami.Units.largeSpacing
    KantePageTitle { page: page }
    // Kante: the page ground is Kante's ground, not the dialog tint KanteScope hands to the theme.
    background: Rectangle { color: KanteStyle.themed ? KanteStyle.backgroundColor : Kirigami.Theme.backgroundColor }

    actions: [
        Kirigami.Action {
            text: i18n("New task…")
            icon.name: "list-add"
            onTriggered: applicationWindow().openTask()
        }
    ]

    Component.onCompleted: {
        store.refreshBatches()
        store.refreshHome()
    }

    property string topic: ""
    property string folder: ""
    property bool showOlder: false
    property string openFinding: ""
    readonly property var topics: [...new Set(store.batches.map(b => b.topic).filter(t => !!t))].sort()
    readonly property real weekAgo: Date.now() - 7 * 86400000

    function matches(b) {
        if (topic && b.topic !== topic) {
            return false
        }
        if (folder && !(b.files || []).some(f => f.path.startsWith(folder + "/"))) {
            return false
        }
        return true
    }

    function allCards(key) {
        if (key === "findings") {
            return (store.home.findings || []).filter(f => !(store.home.open || {})[f.rule])
                .map(f => ({ finding: true, rule: f.rule, title: store.home.titles ? store.home.titles[f.rule] : f.rule, count: f.count, notes: f.paths.length }))
        }
        return store.batches.filter(b => (b.column === key || (key === "working" && b.column === "failed")) && matches(b))
    }

    // Done shows the last seven days unless you ask for more.
    function cards(key) {
        const all = allCards(key)
        return key === "done" && !showOlder ? all.filter(b => new Date(b.updated).getTime() >= weekAgo) : all
    }

    function open(b) {
        store.batchID = b.id
        store.path = ""
        applicationWindow().show("review")
    }

    // ---------- Drag and drop: a finding onto "AI working" creates its batch,
    // a batch onto "Done" accepts all its open changes. ----------

    property var dragging: null

    function startDrag(data, item, pos) {
        dragging = data
        moveDrag(item, pos)
    }

    function moveDrag(item, pos) {
        const p = item.mapToItem(page, pos.x, pos.y)
        proxy.x = p.x - proxy.width / 2
        proxy.y = p.y - proxy.height / 2
    }

    function endDrag() {
        if (dragging) {
            proxy.Drag.drop()
        }
        dragging = null
    }

    function dropOn(key, data) {
        if (key === "working" && data.finding) {
            store.call("fromFinding", { rule: data.rule })
        } else if (key === "done" && !data.finding) {
            acceptAll.batch = data
            acceptAll.open()
        }
    }

    function dropKeys(key) {
        return key === "working" ? ["finding"] : key === "done" ? ["batch"] : ["none"]
    }

    Kirigami.PromptDialog {
        id: acceptAll
        property var batch: null
        title: i18n("Accept all changes?")
        subtitle: batch ? i18np("Every open change of “%2” is accepted and written: one file.",
                                "Every open change of “%2” is accepted and written: %1 files.",
                                (batch.files || []).filter(f => f.status === "open" || f.status === "stale").length, batch.title) : ""
        standardButtons: Kirigami.Dialog.Ok | Kirigami.Dialog.Cancel
        KanteDialogSkin { dialog: acceptAll }
        onAccepted: {
            for (const f of (batch.files || [])) {
                if (f.status === "open" || f.status === "stale") {
                    page.store.call("decideFile", { id: batch.id, path: f.path, decision: "accepted" })
                }
            }
        }
    }

    // What follows the pointer while dragging.
    QQC2.Control {
        id: proxy
        z: 100
        parent: page
        visible: page.dragging !== null
        width: Kirigami.Units.gridUnit * 12
        padding: Kirigami.Units.largeSpacing
        opacity: 0.9
        Drag.active: page.dragging !== null
        Drag.keys: page.dragging ? [page.dragging.finding ? "finding" : "batch"] : []
        Drag.hotSpot.x: width / 2
        Drag.hotSpot.y: height / 2
        background: Surface { selected: true; raised: true }
        contentItem: QQC2.Label {
            text: page.dragging ? page.dragging.title : ""
            font.bold: true
            elide: Text.ElideRight
        }
    }

    header: Flow {
        padding: Kirigami.Units.largeSpacing
        spacing: Kirigami.Units.smallSpacing
        visible: page.topics.length > 1 || (page.store.status.allowed || []).length > 1
        QQC2.Label {
            text: i18n("Topic:")
            color: KanteStyle.mutedTextColor
            height: allChip.height
            verticalAlignment: Text.AlignVCenter
        }
        KanteChip {
            id: allChip
            text: i18nc("all topics", "All")
            checked: page.topic === "" && page.folder === ""
            onClicked: { page.topic = ""; page.folder = "" }
        }
        Repeater {
            model: page.topics
            delegate: KanteChip {
                required property string modelData
                text: modelData
                chipColor: KanteStyle.tagColor
                checked: page.topic === modelData
                onClicked: page.topic = page.topic === modelData ? "" : modelData
            }
        }
        Repeater {
            model: (page.store.status.allowed || []).length > 1 ? page.store.status.allowed : []
            delegate: KanteChip {
                required property string modelData
                text: modelData + "/"
                chipColor: KanteStyle.mutedTextColor
                checked: page.folder === modelData
                onClicked: page.folder = page.folder === modelData ? "" : modelData
            }
        }
    }

    QQC2.ScrollView {
        id: scroll
        anchors.fill: parent
        contentWidth: board.width

        RowLayout {
            id: board
            width: Math.max(implicitWidth, page.width - Kirigami.Units.largeSpacing * 3)
            spacing: Kirigami.Units.largeSpacing

            Repeater {
                model: page.columns
                delegate: Item {
                    id: column
                    required property var modelData
                    readonly property var items: page.cards(modelData.key)
                    readonly property int hiddenOld: modelData.key === "done" && !page.showOlder ? page.allCards("done").length - items.length : 0
                    // Empty columns shrink to a strip, unless something could be dropped there.
                    readonly property bool narrow: items.length === 0 && hiddenOld === 0 && !drop.containsDrag
                        && !(page.dragging && page.dropKeys(modelData.key).indexOf(page.dragging.finding ? "finding" : "batch") >= 0)
                    Layout.fillWidth: !narrow
                    Layout.fillHeight: true
                    Layout.preferredWidth: narrow ? Kirigami.Units.gridUnit * 2 : Kirigami.Units.gridUnit * 13
                    Layout.minimumWidth: narrow ? Kirigami.Units.gridUnit * 2 : Kirigami.Units.gridUnit * 11
                    Layout.maximumWidth: narrow ? Kirigami.Units.gridUnit * 2 : Number.POSITIVE_INFINITY
                    Layout.alignment: Qt.AlignTop
                    implicitHeight: narrow ? strip.implicitHeight : col.implicitHeight

                    DropArea {
                        id: drop
                        anchors.fill: parent
                        keys: page.dropKeys(column.modelData.key)
                        onDropped: page.dropOn(column.modelData.key, page.dragging)
                    }
                    Rectangle {
                        anchors.fill: parent
                        visible: drop.containsDrag
                        color: Qt.alpha(column.modelData.tone, 0.1)
                        border.color: column.modelData.tone
                        border.width: 1
                        radius: KanteStyle.active ? 0 : Kirigami.Units.cornerRadius
                    }

                    // Narrow strip: count and the title written upwards.
                    ColumnLayout {
                        id: strip
                        visible: column.narrow
                        width: parent.width
                        spacing: Kirigami.Units.largeSpacing
                        QQC2.Label {
                            text: "0"
                            font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                            color: KanteStyle.mutedTextColor
                            Layout.alignment: Qt.AlignHCenter
                        }
                        Rectangle { Layout.fillWidth: true; implicitHeight: 2; color: column.modelData.tone }
                        Item {
                            Layout.alignment: Qt.AlignHCenter
                            implicitWidth: vertical.implicitHeight
                            implicitHeight: vertical.implicitWidth
                            KanteSectionLabel {
                                id: vertical
                                text: column.modelData.title
                                color: column.modelData.tone
                                rotation: 90
                                anchors.centerIn: parent
                            }
                        }
                    }

                    ColumnLayout {
                        id: col
                        visible: !column.narrow
                        width: parent.width
                        spacing: Kirigami.Units.smallSpacing

                        RowLayout {
                            KanteSectionLabel { text: column.modelData.title; Layout.fillWidth: true }
                            QQC2.Label { text: column.items.length; font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize); color: KanteStyle.mutedTextColor }
                        }
                        Rectangle { Layout.fillWidth: true; implicitHeight: 2; color: column.modelData.tone }

                        QQC2.Label {
                            visible: drop.containsDrag
                            text: column.modelData.key === "working" ? i18n("Drop to create the batch") : i18n("Drop to accept all changes")
                            color: column.modelData.tone
                            font: Kirigami.Theme.smallFont
                            wrapMode: Text.Wrap
                            Layout.fillWidth: true
                        }

                        Repeater {
                            model: column.items
                            delegate: QQC2.ItemDelegate {
                                id: card
                                required property var modelData
                                readonly property bool draggable: !!modelData.finding || modelData.column === "review" || modelData.column === "feedback"
                                readonly property bool expanded: !!modelData.finding && page.openFinding === modelData.rule
                                Layout.fillWidth: true
                                padding: Kirigami.Units.largeSpacing
                                opacity: page.dragging && (page.dragging.id ? page.dragging.id === modelData.id : page.dragging.rule === modelData.rule) ? 0.4 : 1
                                onClicked: modelData.finding ? page.openFinding = (expanded ? "" : modelData.rule) : page.open(modelData)
                                background: Surface { selected: card.hovered }

                                DragHandler {
                                    enabled: card.draggable
                                    target: null
                                    onActiveChanged: active ? page.startDrag(card.modelData, card, centroid.position) : page.endDrag()
                                    onCentroidChanged: if (active) page.moveDrag(card, centroid.position)
                                }

                                contentItem: ColumnLayout {
                                    spacing: Kirigami.Units.smallSpacing
                                    RowLayout {
                                        visible: !card.modelData.finding
                                        KanteChip {
                                            interactive: false
                                            visible: !!card.modelData.topic
                                            text: card.modelData.topic || ""
                                            chipColor: KanteStyle.tagColor
                                        }
                                        Item { Layout.fillWidth: true }
                                        KanteChip {
                                            interactive: false
                                            visible: card.modelData.questions > 0
                                            text: card.modelData.finding ? "" : i18np("one question", "%1 questions", card.modelData.questions)
                                            chipColor: KanteStyle.neutralTextColor
                                        }
                                    }
                                    RowLayout {
                                        Layout.fillWidth: true
                                        QQC2.Label {
                                            text: card.modelData.title
                                            font.bold: true
                                            wrapMode: Text.Wrap
                                            Layout.fillWidth: true
                                        }
                                        // Findings: the batch action as an icon in the card head.
                                        KanteToolButton {
                                            visible: !!card.modelData.finding
                                            text: i18n("Create batch")
                                            icon.name: "document-new"
                                            display: QQC2.AbstractButton.IconOnly
                                            onClicked: page.store.call("fromFinding", { rule: card.modelData.rule })
                                            QQC2.ToolTip.visible: hovered
                                            QQC2.ToolTip.text: i18n("Create batch (or drag the card onto “AI working”)")
                                        }
                                    }
                                    RowLayout {
                                        visible: !!card.modelData.finding
                                        Kirigami.Icon {
                                            source: card.expanded ? "go-down" : "go-next"
                                            implicitWidth: Kirigami.Units.iconSizes.small
                                            implicitHeight: Kirigami.Units.iconSizes.small
                                        }
                                        QQC2.Label {
                                            text: card.modelData.finding ? i18np("one finding", "%1 findings", card.modelData.count) + " · " + i18np("one note", "%1 notes", card.modelData.notes) : ""
                                            color: KanteStyle.mutedTextColor
                                            Layout.fillWidth: true
                                        }
                                    }
                                    Loader {
                                        active: card.expanded
                                        visible: active
                                        Layout.fillWidth: true
                                        sourceComponent: FindingList { store: page.store; rule: card.modelData.rule; limit: 8 }
                                    }
                                    QQC2.Label {
                                        visible: !card.modelData.finding
                                        // Find-only tasks report notes instead of changes.
                                        text: card.modelData.finding ? "" : card.modelData.found > 0 && card.modelData.counts.files === 0
                                            ? i18np("one note found", "%1 notes found", card.modelData.found)
                                            : i18np("one file", "%1 files", card.modelData.counts.files) + " · " + i18np("one change", "%1 changes", card.modelData.counts.hunks)
                                        color: KanteStyle.mutedTextColor
                                        font: Kirigami.Theme.smallFont
                                    }
                                    QQC2.Label {
                                        visible: !card.modelData.finding && (!!card.modelData.running || !!card.modelData.revising)
                                        text: i18n("AI: %1", page.store.progress[card.modelData.id] || i18n("working…"))
                                        color: KanteStyle.neutralTextColor
                                        font: Kirigami.Theme.smallFont
                                        elide: Text.ElideRight
                                        Layout.fillWidth: true
                                    }
                                    QQC2.Label {
                                        visible: !card.modelData.finding && !!card.modelData.error
                                        text: card.modelData.error || ""
                                        color: KanteStyle.negativeTextColor
                                        wrapMode: Text.Wrap
                                        font: Kirigami.Theme.smallFont
                                        Layout.fillWidth: true
                                    }
                                    KanteProgressBar {
                                        // Review progress of the batch: accepted, rejected, the rest open.
                                        readonly property var counts: card.modelData.counts || ({})
                                        readonly property int open: (counts.hunks || 0) - (counts.accepted || 0) - (counts.rejected || 0)
                                        visible: !card.modelData.finding && counts.hunks > 0
                                        parts: counts.hunks ? [{ value: counts.accepted / counts.hunks, color: KanteStyle.positiveTextColor },
                                                               { value: counts.rejected / counts.hunks, color: KanteStyle.negativeTextColor }] : []
                                        Layout.fillWidth: true
                                        HoverHandler { id: progressHover }
                                        QQC2.ToolTip.visible: progressHover.hovered
                                        QQC2.ToolTip.delay: Kirigami.Units.toolTipDelay
                                        QQC2.ToolTip.text: i18n("%1 accepted · %2 rejected · %3 open", counts.accepted || 0, counts.rejected || 0, open)
                                    }
                                    // Age, provider and cost.
                                    RowLayout {
                                        visible: !card.modelData.finding
                                        QQC2.Label {
                                            text: {
                                                const b = card.modelData
                                                if (b.finding) {
                                                    return ""
                                                }
                                                const u = page.store.usageOf(b)
                                                return [page.store.age(b.column === "done" ? b.updated : b.created), b.provider || "",
                                                        page.store.money(u, page.store.currencyOf(b))].filter(s => !!s).join(" · ")
                                            }
                                            color: KanteStyle.mutedTextColor
                                            font: Kirigami.Theme.smallFont
                                            elide: Text.ElideRight
                                            Layout.fillWidth: true
                                        }
                                        KanteToolButton {
                                            visible: card.modelData.column === "done"
                                            icon.name: "edit-undo"
                                            text: i18n("Undo")
                                            display: QQC2.AbstractButton.IconOnly
                                            onClicked: page.store.call("undoBatch", { id: card.modelData.id })
                                            QQC2.ToolTip.text: text
                                            QQC2.ToolTip.visible: hovered
                                        }
                                        KanteToolButton {
                                            visible: card.modelData.running === true
                                            icon.name: "process-stop"
                                            text: i18n("Stop")
                                            display: QQC2.AbstractButton.IconOnly
                                            onClicked: page.store.call("cancel", { id: card.modelData.id })
                                            QQC2.ToolTip.text: text
                                            QQC2.ToolTip.visible: hovered
                                        }
                                        KanteToolButton {
                                            visible: card.modelData.column === "failed"
                                            icon.name: "edit-delete"
                                            text: i18n("Discard")
                                            display: QQC2.AbstractButton.IconOnly
                                            onClicked: page.store.call("discard", { id: card.modelData.id })
                                            QQC2.ToolTip.text: text
                                            QQC2.ToolTip.visible: hovered
                                        }
                                    }
                                }
                            }
                        }

                        KanteButton {
                            visible: column.hiddenOld > 0 || (column.modelData.key === "done" && page.showOlder)
                            flat: true
                            text: page.showOlder ? i18n("Only the last 7 days") : i18np("Show one older", "Show %1 older", column.hiddenOld)
                            Layout.alignment: Qt.AlignHCenter
                            onClicked: page.showOlder = !page.showOlder
                        }
                    }
                }
            }
        }
    }
}
