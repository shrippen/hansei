import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante
import "components"

/**
 * The review line (Schleifstein): batches on the left, the diff of one file in the middle,
 * the conversation with the AI on the right. Everything works from the keyboard; the side
 * columns can be resized by dragging and keep their width.
 */
Kirigami.Page {
    id: page

    readonly property var store: applicationWindow().store
    readonly property var layout: applicationWindow().layout
    property var batch: null
    property var file: null
    property int hunk: 0
    // Batches waiting for review; the batch you look at stays in the list even when it is done.
    readonly property var reviewable: store.batches.filter(b => b.column === "review" || b.column === "feedback")
    readonly property var listed: {
        const cur = store.batch(store.batchID)
        return cur && !reviewable.some(b => b.id === cur.id) ? [cur].concat(reviewable) : reviewable
    }
    readonly property var questions: batch ? (batch.thread || []).filter(m => m.question && !m.answered) : []

    readonly property bool showList: width > Kirigami.Units.gridUnit * 56
    // The conversation gets its own column only where the diff keeps enough room; it can be folded away.
    readonly property bool showFeedback: width > Kirigami.Units.gridUnit * 60 && !layout.feedbackFolded
    // Two columns of diff need room: below ~700 px the diff is shown unified.
    readonly property bool narrowDiff: width < Kirigami.Units.gridUnit * 38
    readonly property string diffMode: store.mode === "unified" || (store.mode === "split" && narrowDiff) ? "unified" : "split"
    onNarrowDiffChanged: if (store.mode === "split") loadFile()
    readonly property bool keys: page.isCurrentPage && !store.typing && !dialogOpen
    property bool dialogOpen: rejectDialog.opened || editDialog.opened || doneDialog.opened || keysDialog.opened || ruleDialog.opened
    property int context: 3
    property var wrapOff: ({})       // path → lines not wrapped
    property bool renderedBoth: false
    property bool renderedRaw: false

    // Breadcrumb: topic › folder › file.
    title: file ? [batch && batch.topic ? batch.topic : "", file.path.split("/").slice(0, -1).join("/"), store.fileName(file.path)].filter(s => !!s).join("  ›  ")
                : (batch ? batch.title : i18n("Review"))
    padding: 0
    KantePageTitle { page: page }
    // Kante: the page ground is Kante's ground, not the dialog tint KanteScope hands to the theme.
    background: Rectangle { color: KanteStyle.themed ? KanteStyle.backgroundColor : Kirigami.Theme.backgroundColor }

    actions: [
        Kirigami.Action {
            text: i18n("Feedback")
            icon.name: "mail-reply-sender"
            visible: !page.showFeedback
            onTriggered: page.width > Kirigami.Units.gridUnit * 60 ? page.layout.feedbackFolded = false : feedbackDrawer.open()
        },
        Kirigami.Action {
            text: i18n("Accept file")
            icon.name: "dialog-ok-apply"
            visible: page.file !== null && page.file.status === "open"
            enabled: page.file && page.file.hunks.some(h => h.state === "pending")
            onTriggered: page.decideFile("accepted")
            tooltip: i18n("Accept every open change of this file (Shift+A)")
        },
        Kirigami.Action {
            text: i18n("Keys")
            icon.name: "input-keyboard"
            displayHint: Kirigami.DisplayHint.AlwaysHide
            onTriggered: keysDialog.open()
        },
        Kirigami.Action {
            text: page.store.reveal ? i18n("Hide secrets") : i18n("Show all secrets")
            icon.name: page.store.reveal ? "view-hidden" : "view-visible"
            displayHint: Kirigami.DisplayHint.AlwaysHide
            onTriggered: { page.store.reveal = !page.store.reveal; page.loadFile() }
        },
        Kirigami.Action {
            text: i18n("Discard batch")
            icon.name: "edit-delete"
            displayHint: Kirigami.DisplayHint.AlwaysHide
            enabled: page.batch !== null
            onTriggered: page.store.call("discard", { id: page.batch.id })
        }
    ]

    // ---------- Data ----------

    function loadBatch() {
        if (!store.batchID) {
            batch = null
            file = null
            return
        }
        store.call("batch", { id: store.batchID }, r => {
            if (!r) {
                return
            }
            batch = r
            if (!r.files.some(f => f.path === store.path)) {
                const open = r.files.find(f => f.status === "open" || f.status === "stale")
                store.path = (open || r.files[0] || { path: "" }).path
            }
            loadFile()
        })
    }

    property string pendingHunk: ""   // hunk ID to show once the file is loaded

    function loadFile() {
        if (!store.batchID || !store.path) {
            file = null
            return
        }
        store.call("file", { id: store.batchID, path: store.path, mode: page.diffMode, context: page.context, reveal: store.reveal }, r => {
            if (!r) {
                return
            }
            const same = file && file.path === r.path && file.batch === r.batch
            file = r
            if (!same) {
                hunk = firstOpen()
                diff.expanded = {}
                page.context = 3
            }
            if (pendingHunk) {
                const i = r.hunks.findIndex(h => h.id === pendingHunk)
                if (i >= 0) {
                    hunk = i
                }
                pendingHunk = ""
            }
            hunk = Math.max(0, Math.min(hunk, r.hunks.length - 1))
            Qt.callLater(() => diff.showHunk(hunk))
        })
    }

    function pickBatchIfNeeded() {
        if (store.batchID && store.batch(store.batchID)) {
            return
        }
        store.batchID = reviewable.length > 0 ? reviewable[0].id : ""
        store.path = ""
        loadBatch()
    }

    function firstOpen() {
        if (!file) {
            return 0
        }
        const i = file.hunks.findIndex(h => h.state === "pending")
        return i < 0 ? 0 : i
    }

    function nextOpen() {
        const n = file ? file.hunks.length : 0
        for (let i = 1; i <= n; i++) {
            const j = (hunk + i) % n
            if (file.hunks[j].state === "pending") {
                return j
            }
        }
        return hunk
    }

    function current() {
        return file && hunk < file.hunks.length ? file.hunks[hunk] : null
    }

    /** The first changed line of the current change, for the feedback quote. */
    function quoteOf(i) {
        if (!file) {
            return ""
        }
        for (const r of file.rows) {
            if (r.hunk === i && r.kind !== "hunk") {
                const segs = r.new || r.old || []
                const t = segs.map(s => s.t).join("").trim()
                if (t !== "") {
                    return t
                }
            }
        }
        return ""
    }

    /** Opens a file of this batch, optionally at a change (from the thread). */
    function jump(path, hunkID) {
        pendingHunk = hunkID
        if (path === store.path) {
            loadFile()
            return
        }
        store.path = path
        loadFile()
    }

    // ---------- Decisions ----------

    function decide(hunkID, decision, reason) {
        store.call("decide", { id: store.batchID, path: store.path, hunk: hunkID, decision: decision, reason: reason || "" }, r => afterDecide(r))
    }

    function decideFile(decision) {
        store.call("decideFile", { id: store.batchID, path: store.path, decision: decision }, r => afterDecide(r))
    }

    function afterDecide(r) {
        if (!r) {
            return
        }
        if (r.done) {
            doneDialog.outcome = r
            doneDialog.open()
        }
        if (r.written || r.skipped) {
            const next = batch ? batch.files.find(f => f.path !== store.path && (f.status === "open" || f.status === "stale")) : null
            if (next) {
                store.path = next.path
            }
            loadBatch()
            return
        }
        hunk = nextOpen()
        loadFile()
    }

    function undoFile() {
        store.call("undoFile", { id: store.batchID, path: store.path }, () => loadBatch())
    }

    function stepHunk(d) {
        if (!file || file.hunks.length === 0) {
            return
        }
        hunk = Math.max(0, Math.min(file.hunks.length - 1, hunk + d))
        diff.showHunk(hunk)
    }

    function stepFile(d) {
        if (!batch) {
            return
        }
        const i = batch.files.findIndex(f => f.path === store.path) + d
        if (i >= 0 && i < batch.files.length) {
            store.path = batch.files[i].path
            loadFile()
            return
        }
        const bi = reviewable.findIndex(b => b.id === store.batchID) + d
        if (bi >= 0 && bi < reviewable.length) {
            store.batchID = reviewable[bi].id
            store.path = ""
            loadBatch()
        }
    }

    function cycleVersion() {
        if (!file || file.versions.length < 2) {
            return
        }
        const n = file.current > 1 ? file.current - 1 : file.versions.length
        store.call("setVersion", { id: store.batchID, path: store.path, n: n }, () => loadFile())
    }

    function openHistory(i) {
        applicationWindow().pageStack.layers.push(Qt.resolvedUrl("VersionsPage.qml"), { batchID: store.batchID, path: store.path, hunkIndex: i })
    }

    /** Shows one masked value for ten seconds. */
    function revealRow(i) {
        store.call("file", { id: store.batchID, path: store.path, mode: page.diffMode, context: page.context, reveal: true }, r => {
            if (!r || !r.rows[i]) {
                return
            }
            const o = Object.assign({}, diff.overrides)
            o[i] = r.rows[i]
            diff.overrides = o
            hideSecret.restart()
        })
    }
    Timer { id: hideSecret; interval: 10000; onTriggered: diff.overrides = {} }

    function openFeedback(scope, prefix) {
        if (!showFeedback) {
            feedbackDrawer.open()
            drawerPane.compose(scope, prefix)
            return
        }
        feedback.compose(scope, prefix)
    }

    // Sends an answer to one AI question as feedback on the batch, quoting the question.
    function sendAnswer(question, field) {
        const text = field.text.trim()
        if (!batch || text === "") {
            return
        }
        store.call("feedback", { batch: batch.id, scope: "batch", text: "„" + question.text.split("\n")[0] + "“: " + text }, (r, err) => {
            if (!err) {
                field.text = ""
            }
        })
    }

    // Called by the window's toast: true when the message was shown here.
    function showNotice(text, action) {
        if (!bottomBar.visible) {
            return false
        }
        notice.onUndo = action || null
        notice.text = text
        noticeTimer.restart()
        return true
    }

    function answer(question) {
        openFeedback("batch", "„" + question.text.split("\n")[0] + "“: ")
    }

    Component.onCompleted: {
        pickBatchIfNeeded()
        if (store.batchID) {
            loadBatch()
        }
    }
    onReviewableChanged: Qt.callLater(pickBatchIfNeeded)

    Connections {
        target: page.store
        function onBatchChanged(id) {
            if (id === page.store.batchID) {
                page.loadBatch()
            }
        }
    }

    // ---------- Keyboard ----------

    Shortcut { sequence: "A"; enabled: page.keys; onActivated: { const h = page.current(); if (h) page.decide(h.id, "accepted") } }
    Shortcut { sequence: "R"; enabled: page.keys; onActivated: if (page.current()) rejectDialog.openFor(page.hunk) }
    Shortcut { sequence: "E"; enabled: page.keys; onActivated: if (page.current()) editDialog.openFor(page.hunk) }
    Shortcut { sequence: "F"; enabled: page.keys; onActivated: page.openFeedback(page.current() ? "hunk" : "file") }
    Shortcut { sequence: "Shift+F"; enabled: page.keys; onActivated: page.openFeedback("batch") }
    Shortcut { sequence: "J"; enabled: page.keys; onActivated: page.stepHunk(1) }
    Shortcut { sequence: "K"; enabled: page.keys; onActivated: page.stepHunk(-1) }
    Shortcut { sequence: "Down"; enabled: page.keys; onActivated: page.stepHunk(1) }
    Shortcut { sequence: "Up"; enabled: page.keys; onActivated: page.stepHunk(-1) }
    Shortcut { sequence: "Shift+J"; enabled: page.keys; onActivated: page.stepFile(1) }
    Shortcut { sequence: "Shift+K"; enabled: page.keys; onActivated: page.stepFile(-1) }
    Shortcut { sequence: "Shift+A"; enabled: page.keys; onActivated: page.decideFile("accepted") }
    Shortcut { sequence: "Shift+X"; enabled: page.keys; onActivated: page.decideFile("rejected") }
    Shortcut { sequence: "V"; enabled: page.keys; onActivated: page.cycleVersion() }
    Shortcut { sequence: "Shift+V"; enabled: page.keys; onActivated: page.openHistory(page.hunk) }
    Shortcut { sequence: "M"; enabled: page.keys; onActivated: { page.store.mode = page.store.mode === "split" ? "unified" : (page.store.mode === "unified" ? "rendered" : "split"); page.loadFile() } }
    Shortcut { sequence: "U"; enabled: page.keys; onActivated: page.undoFile() }
    Shortcut { sequence: "Shift+R"; enabled: page.keys; onActivated: { const h = page.current(); page.store.call("regenerate", { id: page.store.batchID, path: page.store.path, hunk: h ? h.id : "" }) } }
    Shortcut { sequences: ["?", "Shift+?"]; enabled: page.keys; onActivated: keysDialog.open() }

    // ---------- Layout ----------

    QQC2.SplitView {
        id: split
        anchors.fill: parent
        orientation: Qt.Horizontal

        // Remember the column widths after dragging a handle.
        onResizingChanged: {
            if (resizing) {
                return
            }
            if (list.visible) {
                page.layout.batchListWidth = list.width
            }
            if (feedback.visible) {
                page.layout.feedbackWidth = feedback.width
            }
        }

        BatchList {
            id: list
            visible: page.showList
            QQC2.SplitView.preferredWidth: page.layout.batchListWidth
            QQC2.SplitView.minimumWidth: Kirigami.Units.gridUnit * 10
            QQC2.SplitView.maximumWidth: Kirigami.Units.gridUnit * 30
            batches: page.listed
            batch: page.batch
            selected: page.store.batchID
            path: page.store.path
            progress: page.store.progress
            onPickBatch: id => { page.store.batchID = id; page.store.path = ""; page.loadBatch() }
            onPickFile: p => { page.store.path = p; page.loadFile() }
        }

        ColumnLayout {
            id: center
            QQC2.SplitView.fillWidth: true
            QQC2.SplitView.minimumWidth: Kirigami.Units.gridUnit * 20
            spacing: 0

            // Narrow windows: batch and file pickers with their state instead of the list.
            RowLayout {
                visible: !page.showList && page.listed.length > 0
                Layout.fillWidth: true
                Layout.leftMargin: Kirigami.Units.largeSpacing
                Layout.rightMargin: Kirigami.Units.largeSpacing
                Layout.topMargin: Kirigami.Units.smallSpacing
                QQC2.ComboBox {
                    Layout.fillWidth: true
                    model: page.listed.map(b => {
                        const open = b.counts.hunks - b.counts.accepted - b.counts.rejected
                        return (b.running || b.revising ? "⟳ " : (open === 0 ? "✓ " : "")) + b.title + (open > 0 ? " · " + i18np("one open", "%1 open", open) : "")
                    })
                    currentIndex: page.listed.findIndex(b => b.id === page.store.batchID)
                    onActivated: i => { page.store.batchID = page.listed[i].id; page.store.path = ""; page.loadBatch() }
                    KanteFieldSkin { control: parent }
                }
                QQC2.ComboBox {
                    Layout.fillWidth: true
                    model: page.batch ? page.batch.files.map(f => (f.status === "applied" ? "✓ " : f.status === "stale" ? "⚠ " : (f.status === "skipped" || f.status === "undone") ? "↶ " : "")
                                                           + page.store.fileName(f.path) + (f.open > 0 ? " · " + i18np("one open", "%1 open", f.open) : "")) : []
                    currentIndex: page.batch ? page.batch.files.findIndex(f => f.path === page.store.path) : -1
                    onActivated: i => { page.store.path = page.batch.files[i].path; page.loadFile() }
                    KanteFieldSkin { control: parent }
                }
            }

            // Messages that would hide the bottom action bar as a toast show here instead.
            Kirigami.InlineMessage {
                id: notice
                KanteMessageSkin { message: parent }
                property var onUndo: null
                Layout.fillWidth: true
                Layout.margins: Kirigami.Units.largeSpacing
                Layout.bottomMargin: 0
                visible: text !== ""
                type: Kirigami.MessageType.Positive
                showCloseButton: true
                actions: Kirigami.Action {
                    visible: notice.onUndo !== null
                    text: i18n("Undo")
                    icon.name: "edit-undo"
                    onTriggered: { notice.onUndo(); notice.text = "" }
                }
                Timer { id: noticeTimer; interval: 8000; onTriggered: notice.text = "" }
            }

            // Open questions of the AI come first, each answered right here (Ctrl+Enter sends).
            QQC2.Control {
                visible: page.questions.length > 0
                Layout.fillWidth: true
                Layout.margins: Kirigami.Units.largeSpacing
                Layout.bottomMargin: 0
                padding: Kirigami.Units.largeSpacing
                background: Surface { fill: Qt.alpha(KanteStyle.neutralTextColor, 0.12); selected: true }
                contentItem: ColumnLayout {
                    spacing: Kirigami.Units.largeSpacing
                    RowLayout {
                        Kirigami.Icon {
                            source: "dialog-question"
                            implicitWidth: Kirigami.Units.iconSizes.small
                            implicitHeight: Kirigami.Units.iconSizes.small
                        }
                        KanteSectionLabel {
                            text: i18np("The AI has a question", "The AI has %1 questions", page.questions.length)
                            color: KanteStyle.neutralTextColor
                            Layout.fillWidth: true
                        }
                    }
                    Repeater {
                        model: page.questions
                        delegate: ColumnLayout {
                            id: question
                            required property var modelData
                            Layout.fillWidth: true
                            spacing: Kirigami.Units.smallSpacing
                            QQC2.Label {
                                text: question.modelData.text
                                wrapMode: Text.Wrap
                                Layout.fillWidth: true
                            }
                            RowLayout {
                                Layout.fillWidth: true
                                QQC2.TextArea {
                                    id: answerField
                                    Layout.fillWidth: true
                                    wrapMode: TextEdit.Wrap
                                    placeholderText: i18n("Your answer… (Ctrl+Enter sends)")
                                    onActiveFocusChanged: page.store.typing = activeFocus
                                    Keys.onPressed: event => {
                                        if ((event.key === Qt.Key_Return || event.key === Qt.Key_Enter) && (event.modifiers & Qt.ControlModifier)) {
                                            page.sendAnswer(question.modelData, answerField)
                                            event.accepted = true
                                        }
                                    }
                                    KanteFieldSkin { control: parent }
                                }
                                KanteButton {
                                    text: i18n("Answer")
                                    icon.name: "document-send"
                                    enabled: answerField.text.trim() !== ""
                                    onClicked: page.sendAnswer(question.modelData, answerField)
                                }
                            }
                        }
                    }
                }
            }

            // File header: versions, wrapping, layout.
            RowLayout {
                visible: page.file !== null
                Layout.fillWidth: true
                Layout.margins: Kirigami.Units.largeSpacing
                Layout.bottomMargin: Kirigami.Units.smallSpacing
                spacing: Kirigami.Units.largeSpacing

                KanteToolButton {
                    visible: page.file && page.file.versions.length > 1
                    text: page.file ? i18n("Version %1 of %2", page.file.current, page.file.versions.length) : ""
                    icon.name: "view-history"
                    onClicked: page.openHistory(page.hunk)
                    QQC2.ToolTip.visible: hovered
                    QQC2.ToolTip.text: i18n("All versions of this file (Shift+V)")
                }
                Item { Layout.fillWidth: true }
                KanteToolButton {
                    visible: page.store.mode !== "rendered"
                    text: i18n("Wrap lines")
                    icon.name: "text-wrap"
                    checkable: true
                    checked: page.file ? !page.wrapOff[page.file.path] : true
                    display: QQC2.AbstractButton.IconOnly
                    onToggled: {
                        const w = Object.assign({}, page.wrapOff)
                        w[page.file.path] = !checked
                        page.wrapOff = w
                    }
                    QQC2.ToolTip.visible: hovered
                    QQC2.ToolTip.text: checked ? i18n("Wrap lines (on)") : i18n("Wrap lines (off)")
                }
                KanteToolButton {
                    visible: page.store.mode === "rendered"
                    text: i18n("Before | after")
                    icon.name: "view-split-left-right"
                    checkable: true
                    checked: page.renderedBoth
                    display: center.width > Kirigami.Units.gridUnit * 50 ? QQC2.AbstractButton.TextBesideIcon : QQC2.AbstractButton.IconOnly
                    onToggled: page.renderedBoth = checked
                }
                KanteToolButton {
                    visible: page.store.mode === "rendered"
                    text: i18n("Markdown source")
                    icon.name: "text-x-markdown"
                    checkable: true
                    checked: page.renderedRaw
                    display: center.width > Kirigami.Units.gridUnit * 50 ? QQC2.AbstractButton.TextBesideIcon : QQC2.AbstractButton.IconOnly
                    onToggled: page.renderedRaw = checked
                    QQC2.ToolTip.visible: hovered && display === QQC2.AbstractButton.IconOnly
                    QQC2.ToolTip.text: text
                }
                // The three layouts as one segmented switch.
                QQC2.Control {
                    padding: 1
                    background: Rectangle {
                        color: "transparent"
                        border.width: 1
                        border.color: KanteStyle.frameColor
                        radius: KanteStyle.active ? 0 : Kirigami.Units.cornerRadius
                    }
                    contentItem: RowLayout {
                        spacing: 0
                        QQC2.ButtonGroup { id: modes }
                        Repeater {
                            model: [["split", i18n("Side by side"), "view-split-left-right"], ["unified", i18n("Unified"), "view-list-text"], ["rendered", i18n("Rendered"), "view-preview"]]
                            delegate: KanteToolButton {
                                required property var modelData
                                text: modelData[1]
                                icon.name: modelData[2]
                                display: center.width > Kirigami.Units.gridUnit * 30 ? QQC2.AbstractButton.TextOnly : QQC2.AbstractButton.IconOnly
                                checkable: true
                                checked: page.store.mode === modelData[0]
                                QQC2.ButtonGroup.group: modes
                                onClicked: { page.store.mode = modelData[0]; page.loadFile() }
                                QQC2.ToolTip.visible: hovered && display === QQC2.AbstractButton.IconOnly
                                QQC2.ToolTip.text: text
                            }
                        }
                    }
                }
            }

            // What the AI did and why.
            QQC2.Control {
                visible: page.file && (page.file.summary || page.batch && page.batch.summary)
                Layout.fillWidth: true
                Layout.leftMargin: Kirigami.Units.largeSpacing
                Layout.rightMargin: Kirigami.Units.largeSpacing
                Layout.bottomMargin: Kirigami.Units.smallSpacing
                padding: Kirigami.Units.largeSpacing
                background: Surface {}
                contentItem: QQC2.Label {
                    text: page.file ? (page.file.summary || page.batch.summary) : ""
                    wrapMode: Text.Wrap
                }
            }

            Kirigami.InlineMessage {
                KanteMessageSkin { message: parent }
                Layout.fillWidth: true
                Layout.margins: Kirigami.Units.largeSpacing
                visible: page.file && page.file.stale
                type: Kirigami.MessageType.Warning
                text: i18n("This note changed since the proposal. Changes that still fit are placed into the new content when you accept.")
                actions: Kirigami.Action {
                    text: i18n("Propose again")
                    icon.name: "view-refresh"
                    onTriggered: page.store.call("regenerate", { id: page.store.batchID, path: page.store.path, hunk: "" })
                }
            }
            Kirigami.InlineMessage {
                KanteMessageSkin { message: parent }
                Layout.fillWidth: true
                Layout.margins: Kirigami.Units.largeSpacing
                visible: page.file && page.file.status !== "open"
                type: page.file && page.file.status === "applied" ? Kirigami.MessageType.Positive : Kirigami.MessageType.Information
                text: !page.file ? "" : page.file.status === "applied" ? i18n("Written to the vault.")
                    : page.file.status === "skipped" ? i18n("Everything rejected, nothing written.") : i18n("Reverted.")
                actions: [
                    Kirigami.Action {
                        visible: page.file && page.file.status === "applied"
                        text: i18n("Undo")
                        icon.name: "edit-undo"
                        onTriggered: page.undoFile()
                    },
                    Kirigami.Action {
                        visible: page.file && page.file.status !== "applied"
                        text: i18n("Review again")
                        icon.name: "view-refresh"
                        onTriggered: page.store.call("reopen", { id: page.store.batchID, path: page.store.path }, () => page.loadBatch())
                    }
                ]
            }
            Kirigami.InlineMessage {
                KanteMessageSkin { message: parent }
                Layout.fillWidth: true
                Layout.margins: Kirigami.Units.largeSpacing
                visible: page.store.reveal
                type: Kirigami.MessageType.Warning
                text: i18n("Secrets are visible.")
            }

            DiffView {
                id: diff
                visible: page.store.mode !== "rendered" && page.file !== null
                Layout.fillWidth: true
                Layout.fillHeight: true
                file: page.file
                currentHunk: page.hunk
                wrap: page.file ? !page.wrapOff[page.file.path] : true
                onDecide: (id, d) => page.decide(id, d)
                onReject: i => rejectDialog.openFor(i)
                onEdit: i => editDialog.openFor(i)
                onFeedback: i => { page.hunk = i; page.openFeedback("hunk") }
                onHistory: i => page.openHistory(i)
                onRegenerate: i => page.store.call("regenerate", { id: page.store.batchID, path: page.store.path, hunk: page.file.hunks[i].id })
                onShowAll: { page.context = -1; page.loadFile() }
                onPick: i => { page.hunk = i }
                onRule: ref => ruleDialog.show(ref)
                onRevealRow: i => page.revealRow(i)
                onLineFeedback: (h, line, old) => {
                    if (h >= 0) {
                        page.hunk = h
                    }
                    page.openFeedback(h >= 0 ? "hunk" : "file", old ? i18n("Old line %1: ", line) : i18n("Line %1: ", line))
                }
            }

            RenderedView {
                visible: page.store.mode === "rendered" && page.file !== null
                Layout.fillWidth: true
                Layout.fillHeight: true
                file: page.file
                store: page.store
                sideBySide: page.renderedBoth
                raw: page.renderedRaw
            }

            // A find-only task: the notes the AI found, each opens in Obsidian.
            ColumnLayout {
                visible: !page.file && page.batch !== null && (page.batch.found || []).length > 0
                Layout.fillWidth: true
                Layout.margins: Kirigami.Units.largeSpacing
                Kirigami.Heading {
                    level: 3
                    text: page.batch ? i18np("The AI found one note", "The AI found %1 notes", (page.batch.found || []).length) : ""
                    font: KanteStyle.headingFont(Kirigami.Theme.defaultFont.pointSize * 1.2)
                }
                QQC2.Label {
                    text: page.batch ? page.batch.instruction : ""
                    color: KanteStyle.mutedTextColor
                    wrapMode: Text.Wrap
                    Layout.fillWidth: true
                }
                FindingList {
                    Layout.fillWidth: true
                    store: page.store
                    found: page.batch ? (page.batch.found || []) : []
                    limit: 200
                }
                RowLayout {
                    KanteButton {
                        text: i18n("Propose changes for these notes…")
                        icon.name: "document-new"
                        emphasis: KanteButton.Emphasis.Primary
                        onClicked: applicationWindow().openTask(page.batch.instruction, null)
                    }
                    KanteButton {
                        text: i18n("Discard")
                        icon.name: "edit-delete"
                        onClicked: page.store.call("discard", { id: page.batch.id })
                    }
                }
                Item { Layout.fillHeight: true }
            }

            Kirigami.PlaceholderMessage {
                visible: !page.file && !(page.batch && (page.batch.found || []).length > 0)
                Layout.fillWidth: true
                Layout.fillHeight: true
                icon.name: page.batch && (page.batch.running || page.batch.status === "working") ? "view-refresh" : "document-compare"
                text: !page.batch ? i18n("Nothing waiting for review") : (page.batch.running ? i18n("The AI is working on this batch") : page.batch.title)
                explanation: !page.batch ? i18n("New proposals appear here. Press N for a new task.") : (page.batch.error || page.batch.summary || "")
                helpfulAction: Kirigami.Action {
                    text: i18n("New task…")
                    icon.name: "list-add"
                    onTriggered: applicationWindow().openTask()
                }
            }

            // Narrow windows: the actions for the current change stay at the bottom.
            QQC2.ToolBar {
                id: bottomBar
                visible: !page.showFeedback && page.file !== null && page.file.status === "open" && page.current() !== null && page.current().state === "pending"
                Layout.fillWidth: true
                position: QQC2.ToolBar.Footer
                contentItem: RowLayout {
                    KanteButton {
                        text: i18n("Accept")
                        icon.name: "dialog-ok-apply"
                        emphasis: KanteButton.Emphasis.Primary
                        Layout.fillWidth: true
                        onClicked: { const h = page.current(); if (h) page.decide(h.id, "accepted") }
                    }
                    KanteButton {
                        text: i18n("Reject")
                        icon.name: "dialog-cancel"
                        Layout.fillWidth: true
                        onClicked: rejectDialog.openFor(page.hunk)
                    }
                    KanteButton {
                        text: i18n("Feedback")
                        icon.name: "mail-reply-sender"
                        Layout.fillWidth: true
                        onClicked: page.openFeedback("hunk")
                    }
                }
            }

            // The most important keys; ? shows all. After a few sessions only the way to the overview stays.
            RowLayout {
                Layout.fillWidth: true
                Layout.margins: Kirigami.Units.smallSpacing
                visible: page.file !== null && page.showFeedback
                spacing: Kirigami.Units.largeSpacing
                Repeater {
                    model: page.layout.sessions > 5 && !Hansei.demoBuild ? [["?", i18n("all keys")]]
                        : [["A", i18n("accept")], ["R", i18n("reject")], ["F", i18n("feedback")], ["J/K", i18n("next/previous")], ["?", i18n("all keys")]]
                    delegate: RowLayout {
                        required property var modelData
                        spacing: Kirigami.Units.smallSpacing
                        QQC2.Label {
                            text: modelData[0]
                            font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize, true)
                            leftPadding: Kirigami.Units.smallSpacing
                            rightPadding: Kirigami.Units.smallSpacing
                            background: Rectangle {
                                color: KanteStyle.sunkenColor
                                border.width: 1
                                border.color: KanteStyle.frameColor
                                radius: KanteStyle.active ? 0 : 3
                            }
                        }
                        QQC2.Label {
                            text: modelData[1]
                            font: Kirigami.Theme.smallFont
                            color: KanteStyle.mutedTextColor
                        }
                    }
                }
                Item { Layout.fillWidth: true }
                TapHandler { onTapped: keysDialog.open() }
            }
        }

        FeedbackPane {
            id: feedback
            visible: page.showFeedback
            QQC2.SplitView.preferredWidth: page.layout.feedbackWidth
            QQC2.SplitView.minimumWidth: Kirigami.Units.gridUnit * 14
            QQC2.SplitView.maximumWidth: Kirigami.Units.gridUnit * 34
            store: page.store
            batch: page.batch
            file: page.file
            hunkIndex: page.hunk
            quote: page.quoteOf(page.hunk)
            foldable: true
            onFold: page.layout.feedbackFolded = true
            onJump: (path, hunkID) => page.jump(path, hunkID)
        }
    }

    QQC2.Drawer {
        id: feedbackDrawer
        parent: applicationWindow().overlay
        edge: Qt.RightEdge
        modal: true
        width: Math.min(applicationWindow().width, Kirigami.Units.gridUnit * 22)
        height: applicationWindow().height
        contentItem: FeedbackPane {
            id: drawerPane
            store: page.store
            batch: page.batch
            file: page.file
            hunkIndex: page.hunk
            quote: page.quoteOf(page.hunk)
            onJump: (path, hunkID) => { feedbackDrawer.close(); page.jump(path, hunkID) }
        }
    }

    RejectDialog {
        id: rejectDialog
        onRejectHunk: (hunkIndex, reason, again, quick) => {
            const h = page.file.hunks[hunkIndex]
            page.decide(h.id, "rejected", reason)
            if (again) {
                page.store.call("feedback", { batch: page.store.batchID, scope: "hunk", path: page.store.path, hunk: h.id, text: reason || i18n("Please propose this change again."), quick: quick })
            }
        }
        function openFor(i) {
            hunkIndex = i
            open()
        }
    }

    EditDialog {
        id: editDialog
        store: page.store
        onSaved: page.loadBatch()
        function openFor(i) {
            load(page.store.batchID, page.store.path, page.file.hunks[i], i)
        }
    }

    DoneDialog {
        id: doneDialog
        store: page.store
        onNext: {
            const next = page.reviewable.find(b => b.id !== page.store.batchID && b.column === "review")
            if (next) {
                page.store.batchID = next.id
                page.store.path = ""
                page.loadBatch()
                return
            }
            applicationWindow().show("start")
        }
    }

    KeysDialog { id: keysDialog }

    // A rule from the rulebook, opened from a change's rule chip.
    Kirigami.Dialog {
        id: ruleDialog
        property var section: null
        title: section ? (section.heading ? section.heading.replace(/^#+\s*/, "") : section.file) : ""
        preferredWidth: Math.min(Kirigami.Units.gridUnit * 30, applicationWindow().width - Kirigami.Units.gridUnit * 4)
        preferredHeight: Math.min(ruleForm.implicitHeight + topPadding + bottomPadding + Kirigami.Units.gridUnit * 5, applicationWindow().height - Kirigami.Units.gridUnit * 4)
        padding: Kirigami.Units.largeSpacing
        standardButtons: Kirigami.Dialog.Close
        function show(ref) {
            page.store.call("ruleSection", { ref: ref }, r => {
                if (r) {
                    section = r
                    open()
                }
            })
        }
        ColumnLayout {
            id: ruleForm
            KanteSectionLabel { text: ruleDialog.section ? ruleDialog.section.file : "" }
            QQC2.Label {
                Layout.fillWidth: true
                text: ruleDialog.section ? ruleDialog.section.text : ""
                textFormat: Text.MarkdownText
                wrapMode: Text.Wrap
            }
        }
        KanteDialogSkin { dialog: ruleDialog }
    }
}
