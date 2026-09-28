import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante

/**
 * Talk to the AI about the batch: rule suggestions on top, the thread, and the composer.
 * Feedback is scoped to the current change, the file or the whole batch. After every
 * round a system line says which files got a new version.
 */
ColumnLayout {
    id: pane

    property var store
    property var batch: null
    property var file: null
    property int hunkIndex: -1
    property string scope: "hunk"
    property string quick: ""
    property string filter: "all"   // all, file, hunk
    // First changed line of the current change, quoted above the input.
    property string quote: ""

    signal jump(string path, string hunkID)
    signal fold()
    property bool foldable: false

    readonly property var hunk: file && hunkIndex >= 0 && hunkIndex < file.hunks.length ? file.hunks[hunkIndex] : null
    readonly property bool busy: batch ? (batch.running || batch.revising) : false
    readonly property var suggestions: batch ? (batch.suggestions || []).filter(s => s.status === "open") : []
    readonly property var messages: {
        const all = batch ? (batch.thread || []) : []
        if (filter === "file" && file) {
            return all.filter(m => m.path === file.path || (m.versions && m.versions[file.path] !== undefined))
        }
        if (filter === "hunk" && hunk) {
            return all.filter(m => m.hunk === hunk.id)
        }
        return all
    }

    spacing: 0

    /** Opens the composer for a scope, e.g. from a hunk's Feedback button or the F key; prefix starts the text. */
    function compose(newScope, prefix) {
        scope = newScope
        if (prefix) {
            input.text = prefix
        }
        input.forceActiveFocus()
        input.cursorPosition = input.length
    }

    function send() {
        if (!batch || (input.text.trim() === "" && quick === "")) {
            return
        }
        const params = { batch: batch.id, scope: scope, text: input.text, quick: quick, remember: remember.checked }
        if (scope !== "batch" && file) {
            params.path = file.path
        }
        if (scope === "hunk" && hunk) {
            params.hunk = hunk.id
        }
        store.call("feedback", params, (r, err) => {
            if (!err) {
                input.text = ""
                quick = ""
                remember.checked = false
            }
        })
    }

    function hunkNumber(path, id) {
        if (!file || path !== file.path) {
            return -1
        }
        return file.hunks.findIndex(h => h.id === id)
    }

    function quickLabel(code) {
        switch (code) {
        case "wrong-fact": return i18n("Wrong fact")
        case "too-long": return i18n("Too long")
        case "rulebook": return i18n("Against the rulebook")
        case "question": return i18n("Question")
        }
        return code
    }

    RowLayout {
        Layout.fillWidth: true
        Layout.margins: Kirigami.Units.largeSpacing
        Layout.bottomMargin: Kirigami.Units.smallSpacing
        Kirigami.Heading {
            text: i18n("Feedback to the AI")
            level: 4
            font: KanteStyle.headingFont(Kirigami.Theme.defaultFont.pointSize * 1.05)
            Layout.fillWidth: true
            elide: Text.ElideRight
        }
        QQC2.ComboBox {
            id: filterBox
            flat: true
            model: [i18nc("thread filter", "All"), i18nc("thread filter", "This file"), i18nc("thread filter", "This change")]
            currentIndex: ["all", "file", "hunk"].indexOf(pane.filter)
            onActivated: i => pane.filter = ["all", "file", "hunk"][i]
            QQC2.ToolTip.visible: hovered
            QQC2.ToolTip.text: i18n("Which messages to show")
        }
        KanteToolButton {
            visible: pane.foldable
            icon.name: "sidebar-collapse-right"
            text: i18n("Hide the conversation")
            display: QQC2.AbstractButton.IconOnly
            onClicked: pane.fold()
            QQC2.ToolTip.visible: hovered
            QQC2.ToolTip.text: text
        }
    }

    // Rule suggestions stay on top until you decide: one line, opened on demand.
    Repeater {
        model: pane.suggestions
        delegate: QQC2.Control {
            id: sugg
            required property var modelData
            property bool open: false
            // The quoted feedback without its closing mark: the sentence adds its own.
            readonly property string quoted: modelData.text.trim().replace(/[.!?…]+$/, "")
            Layout.fillWidth: true
            Layout.leftMargin: Kirigami.Units.largeSpacing
            Layout.rightMargin: Kirigami.Units.largeSpacing
            Layout.bottomMargin: Kirigami.Units.smallSpacing
            padding: Kirigami.Units.smallSpacing
            background: Surface { fill: Qt.alpha(KanteStyle.infoColor, 0.1) }
            contentItem: ColumnLayout {
                spacing: Kirigami.Units.smallSpacing
                RowLayout {
                    Layout.fillWidth: true
                    Kirigami.Icon {
                        source: "help-hint"
                        implicitWidth: Kirigami.Units.iconSizes.small
                        implicitHeight: Kirigami.Units.iconSizes.small
                        color: KanteStyle.infoColor
                        isMask: true
                    }
                    QQC2.Label {
                        text: i18n("Rule suggestion: “%1”", sugg.quoted)
                        elide: Text.ElideRight
                        Layout.fillWidth: true
                    }
                    KanteToolButton {
                        text: sugg.open ? i18n("Less") : i18n("Show")
                        onClicked: sugg.open = !sugg.open
                    }
                }
                QQC2.Label {
                    visible: sugg.open
                    text: i18np("You gave this feedback once: “%2”. Make it a rule?", "You gave similar feedback %1 times: “%2”. Make it a rule?", sugg.modelData.count, sugg.quoted)
                        + (sugg.modelData.target ? "\n" + i18n("→ %1", sugg.modelData.target) : "")
                    wrapMode: Text.Wrap
                    Layout.fillWidth: true
                }
                RowLayout {
                    visible: sugg.open
                    Item { Layout.fillWidth: true }
                    KanteButton {
                        text: i18n("Propose as batch")
                        icon.name: "list-add"
                        onClicked: pane.store.call("suggestion", { id: pane.batch.id, sid: sugg.modelData.id, accept: true })
                    }
                    KanteToolButton {
                        text: i18n("No")
                        icon.name: "dialog-cancel"
                        onClicked: pane.store.call("suggestion", { id: pane.batch.id, sid: sugg.modelData.id, accept: false })
                    }
                }
            }
        }
    }

    Kirigami.Separator { Layout.fillWidth: true }

    ListView {
        id: thread
        Layout.fillWidth: true
        Layout.fillHeight: true
        clip: true
        spacing: Kirigami.Units.largeSpacing
        topMargin: Kirigami.Units.largeSpacing
        bottomMargin: Kirigami.Units.largeSpacing
        leftMargin: Kirigami.Units.largeSpacing
        rightMargin: Kirigami.Units.largeSpacing
        model: pane.messages
        onCountChanged: Qt.callLater(positionViewAtEnd)
        QQC2.ScrollBar.vertical: QQC2.ScrollBar {}

        delegate: Loader {
            required property var modelData
            width: ListView.view.width - Kirigami.Units.largeSpacing * 2
            sourceComponent: modelData.role === "system" ? systemLine : bubble
            property var msg: modelData
        }

        footer: ColumnLayout {
            width: thread.width - Kirigami.Units.largeSpacing * 2
            spacing: Kirigami.Units.largeSpacing

            Kirigami.PlaceholderMessage {
                visible: pane.messages.length === 0 && !pane.busy
                Layout.fillWidth: true
                Layout.topMargin: Kirigami.Units.gridUnit
                icon.name: "mail-reply-sender"
                text: pane.filter === "all" ? i18n("No messages yet") : i18n("No messages here")
                explanation: pane.filter === "all" ? i18n("Tell the AI what to do differently. It revises the proposal, you review again.") : ""
            }

            // Streamed text while the AI works.
            QQC2.Control {
                visible: pane.busy
                Layout.fillWidth: true
                Layout.topMargin: Kirigami.Units.largeSpacing
                padding: Kirigami.Units.largeSpacing
                background: Surface { fill: KanteStyle.sunkenColor; bar: KanteStyle.accentColor }
                contentItem: ColumnLayout {
                    RowLayout {
                        QQC2.BusyIndicator { running: pane.busy; Layout.preferredHeight: Kirigami.Units.iconSizes.small; Layout.preferredWidth: Layout.preferredHeight }
                        SectionLabel { text: i18n("AI is working"); Layout.fillWidth: true }
                        KanteToolButton {
                            icon.name: "process-stop"
                            text: i18n("Stop")
                            onClicked: pane.store.call("cancel", { id: pane.batch.id })
                        }
                    }
                    QQC2.Label {
                        text: pane.store.progress[pane.batch ? pane.batch.id : ""] || ""
                        visible: text !== ""
                        font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                        color: KanteStyle.mutedTextColor
                        elide: Text.ElideRight
                        Layout.fillWidth: true
                    }
                    QQC2.Label {
                        text: pane.store.aiText[pane.batch ? pane.batch.id : ""] || ""
                        visible: text !== ""
                        wrapMode: Text.Wrap
                        Layout.fillWidth: true
                    }
                }
            }
        }
    }

    // A round's result: which files got a new version. Click a file to open it.
    Component {
        id: systemLine
        ColumnLayout {
            id: sys
            readonly property var m: parent.msg
            spacing: Kirigami.Units.smallSpacing / 2
            RowLayout {
                Layout.fillWidth: true
                Kirigami.Separator { Layout.fillWidth: true }
                QQC2.Label {
                    text: Qt.formatTime(new Date(sys.m.created), Qt.DefaultLocaleShortDate)
                    font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                    color: KanteStyle.mutedTextColor
                }
                Kirigami.Separator { Layout.fillWidth: true }
            }
            Flow {
                Layout.fillWidth: true
                spacing: Kirigami.Units.smallSpacing
                Repeater {
                    model: Object.keys(sys.m.versions || {})
                    delegate: Chip {
                        required property string modelData
                        interactive: true
                        checkable: false
                        text: i18n("%1 → v%2", pane.store.fileName(modelData), sys.m.versions[modelData])
                        tone: KanteStyle.accentTextColor
                        onClicked: pane.jump(modelData, "")
                        QQC2.ToolTip.visible: hovered
                        QQC2.ToolTip.text: modelData
                    }
                }
            }
            QQC2.Label {
                text: sys.m.text
                font: Kirigami.Theme.smallFont
                color: KanteStyle.mutedTextColor
                wrapMode: Text.Wrap
                horizontalAlignment: Text.AlignHCenter
                Layout.fillWidth: true
            }
        }
    }

    Component {
        id: bubble
        QQC2.Control {
            id: box
            // Your own messages sit a little to the right, like in a chat.
            leftInset: mine ? Kirigami.Units.gridUnit : 0
            leftPadding: Kirigami.Units.largeSpacing + leftInset
            readonly property var m: parent.msg
            readonly property bool mine: m.role === "user"
            readonly property int number: pane.hunkNumber(m.path, m.hunk)
            padding: Kirigami.Units.largeSpacing
            background: Surface {
                fill: box.mine ? (KanteStyle.active ? KanteStyle.cardColor : Kirigami.Theme.alternateBackgroundColor) : KanteStyle.sunkenColor
            }
            contentItem: ColumnLayout {
                spacing: Kirigami.Units.smallSpacing
                RowLayout {
                    Layout.fillWidth: true
                    SectionLabel {
                        text: box.mine ? i18n("You") : i18n("AI")
                    }
                    Chip {
                        visible: !!box.m.path && (box.m.scope === "hunk" || box.m.scope === "file")
                        interactive: true
                        checkable: false
                        text: box.number >= 0 ? i18n("Change %1", box.number + 1) : pane.store.fileName(box.m.path)
                        tone: KanteStyle.infoColor
                        onClicked: pane.jump(box.m.path, box.m.hunk || "")
                        QQC2.ToolTip.visible: hovered
                        QQC2.ToolTip.text: box.m.path || ""
                    }
                    Chip {
                        visible: !!box.m.quick
                        text: pane.quickLabel(box.m.quick)
                        tone: KanteStyle.infoColor
                    }
                    Chip {
                        visible: !!box.m.question && !box.m.answered
                        text: i18n("question")
                        tone: KanteStyle.neutralTextColor
                    }
                    Item { Layout.fillWidth: true }
                    QQC2.Label {
                        text: Qt.formatTime(new Date(box.m.created), Qt.DefaultLocaleShortDate)
                        font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                        color: KanteStyle.mutedTextColor
                    }
                }
                QQC2.Label {
                    text: box.m.text
                    wrapMode: Text.Wrap
                    textFormat: Text.PlainText
                    Layout.fillWidth: true
                }
                Flow {
                    visible: !box.mine && Object.keys(box.m.versions || {}).length > 0
                    Layout.fillWidth: true
                    spacing: Kirigami.Units.smallSpacing
                    Repeater {
                        model: Object.keys(box.m.versions || {})
                        delegate: Chip {
                            required property string modelData
                            interactive: true
                            checkable: false
                            text: i18n("→ v%1 %2", box.m.versions[modelData], pane.store.fileName(modelData))
                            tone: KanteStyle.accentTextColor
                            onClicked: pane.jump(modelData, "")
                        }
                    }
                }
            }
        }
    }

    Kirigami.Separator { Layout.fillWidth: true }

    ColumnLayout {
        Layout.fillWidth: true
        Layout.margins: Kirigami.Units.largeSpacing
        spacing: Kirigami.Units.smallSpacing

        RowLayout {
            Layout.fillWidth: true
            spacing: 0
            QQC2.ButtonGroup { id: scopes }
            Repeater {
                model: [["hunk", pane.hunk ? i18nc("feedback scope", "Change %1", pane.hunk.index + 1) : i18nc("feedback scope", "Change")],
                        ["file", i18nc("feedback scope", "File")], ["batch", i18nc("feedback scope", "Batch")]]
                delegate: KanteToolButton {
                    required property var modelData
                    text: modelData[1]
                    checkable: true
                    checked: pane.scope === modelData[0]
                    enabled: modelData[0] !== "hunk" || pane.hunk !== null
                    QQC2.ButtonGroup.group: scopes
                    onClicked: pane.scope = modelData[0]
                }
            }
            Item { Layout.fillWidth: true }
            KanteToolButton {
                text: pane.quick ? pane.quickLabel(pane.quick) + "  ✕" : i18n("Reason")
                icon.name: pane.quick ? "" : "tag"
                onClicked: pane.quick ? pane.quick = "" : reasons.popup()
                QQC2.Menu {
                    KantePopupSkin { popup: reasons }
                    id: reasons
                    Repeater {
                        model: ["wrong-fact", "too-long", "rulebook", "question"]
                        delegate: QQC2.MenuItem {
                            required property string modelData
                            text: pane.quickLabel(modelData)
                            onTriggered: pane.quick = modelData
                        }
                    }
                }
            }
        }

        // What the feedback refers to.
        QQC2.Label {
            visible: pane.scope === "hunk" && pane.quote !== ""
            text: "“" + pane.quote + "”"
            font.pointSize: Kirigami.Theme.smallFont.pointSize
            font.italic: true
            color: KanteStyle.mutedTextColor
            elide: Text.ElideRight
            HoverHandler { id: quoteHover }
            QQC2.ToolTip.visible: quoteHover.hovered && truncated
            QQC2.ToolTip.text: pane.quote
            Layout.fillWidth: true
            leftPadding: Kirigami.Units.smallSpacing
            Rectangle { width: 2; height: parent.height; color: KanteStyle.infoColor }
        }

        QQC2.TextArea {
            KanteFieldSkin { control: parent }
            id: input
            Layout.fillWidth: true
            Layout.preferredHeight: activeFocus || text !== "" ? Kirigami.Units.gridUnit * 4 : implicitHeight
            wrapMode: TextEdit.Wrap
            enabled: pane.batch !== null && !pane.busy
            placeholderText: pane.scope === "hunk" && pane.hunk ? i18n("Feedback on change %1…", pane.hunk.index + 1)
                : pane.scope === "file" ? i18n("Feedback on this file…") : i18n("Feedback on the whole batch…")
            onActiveFocusChanged: pane.store.typing = activeFocus
            Keys.onPressed: event => {
                if ((event.key === Qt.Key_Return || event.key === Qt.Key_Enter) && !(event.modifiers & Qt.ShiftModifier)) {
                    pane.send()
                    event.accepted = true
                } else if (event.key === Qt.Key_Escape) {
                    input.focus = false
                    event.accepted = true
                }
            }
        }

        RowLayout {
            QQC2.CheckBox {
                id: remember
                text: i18n("Remember as rule")
                KanteCheckSkin { control: parent }
            }
            Item { Layout.fillWidth: true }
            KanteButton {
                text: i18n("Send")
                icon.name: "document-send"
                emphasis: KanteButton.Emphasis.Primary
                enabled: input.enabled && (input.text.trim() !== "" || pane.quick !== "")
                onClicked: pane.send()
            }
        }

        RowLayout {
            Layout.fillWidth: true
            QQC2.Label {
                Layout.fillWidth: true
                font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                color: KanteStyle.mutedTextColor
                elide: Text.ElideRight
                text: {
                    const b = pane.batch
                    const p = pane.store.defaultProvider
                    if (!b) {
                        return ""
                    }
                    const u = pane.store.usageOf(b)
                    const parts = [(b.provider || (p ? p.name : "")) + (p ? " · " + p.model : "")]
                    if (u && u.in + u.out > 0) {
                        parts.push(pane.store.tokens(u))
                    }
                    const money = pane.store.money(u, pane.store.currencyOf(b))
                    if (money) {
                        parts.push(money)
                    }
                    return parts.join(" · ")
                }
            }
            // No price, no cost: say where to add it.
            KanteToolButton {
                readonly property var prov: pane.store.providers.find(p => p.name === (pane.batch && pane.batch.provider ? pane.batch.provider : (pane.store.defaultProvider || {}).name))
                visible: pane.batch !== null && !!prov && !prov.local && !prov.priceIn
                text: i18n("Price missing")
                font: Kirigami.Theme.smallFont
                onClicked: pane.store.window.openSettings("providers")
            }
        }
    }
}
