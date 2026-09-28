import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante
import "components"

/** Start: where work pays off, what waits for you, and a new task in one sentence. */
Kirigami.ScrollablePage {
    id: page

    readonly property var store: applicationWindow().store
    readonly property var stats: store.home.stats || {}
    readonly property var waitingBatches: store.batches.filter(b => b.column === "review" || b.column === "feedback")
    readonly property var findings: store.home.findings || []
    readonly property int maxCount: Math.max(1, ...findings.map(f => f.count))
    property string expanded: ""
    property var estimate: null
    property var quickScope: []
    property var lastWrite: null
    // The check with the most findings: the biggest lever for the rule quota.
    readonly property var lever: findings.length > 0 ? findings.reduce((a, b) => b.count > a.count ? b : a) : null
    // Tiles: five on wide pages, the three essential ones otherwise.
    readonly property int tileColumns: width > Kirigami.Units.gridUnit * 70 ? 5 : (width > Kirigami.Units.gridUnit * 40 ? 3 : 1)
    readonly property var monthUsage: {
        const start = new Date()
        start.setDate(1)
        start.setHours(0, 0, 0, 0)
        const u = { in: 0, out: 0, cost: 0, tasks: 0 }
        for (const b of store.batches) {
            if (new Date(b.created) >= start && (b.usage.in + b.usage.out) > 0) {
                u.in += b.usage.in
                u.out += b.usage.out
                u.cost += b.usage.cost || 0
                u.tasks++
            }
        }
        return u
    }
    readonly property var oldestWaiting: waitingBatches.length > 0 ? waitingBatches.reduce((a, b) => new Date(b.created) < new Date(a.created) ? b : a) : null
    readonly property var recentTasks: {
        try {
            return JSON.parse(applicationWindow().layout.recentTasks || "[]")
        } catch (e) {
            return []
        }
    }

    readonly property bool narrow: width < Kirigami.Units.gridUnit * 38

    function countText(f) {
        return f.count === f.paths.length ? i18np("one finding", "%1 findings", f.count)
            : i18n("%1 in %2", i18np("one finding", "%1 findings", f.count), i18np("one note", "%1 notes", f.paths.length))
    }

    // Keeps the last three task sentences for the chips under the field.
    function remember(text) {
        const list = [text].concat(recentTasks.filter(t => t !== text)).slice(0, 3)
        applicationWindow().layout.recentTasks = JSON.stringify(list)
    }

    title: i18n("Start")
    KantePageTitle { page: page }
    // Kante: the page ground is Kante's ground, not the dialog tint KanteScope hands to the theme.
    background: Rectangle { color: KanteStyle.themed ? KanteStyle.backgroundColor : Kirigami.Theme.backgroundColor }

    Component.onCompleted: {
        store.refreshHome()
        store.call("journal", { limit: 1 }, r => { page.lastWrite = r && r.length > 0 ? r[0] : null })
    }

    function openBatch(id) {
        store.batchID = id
        store.path = ""
        applicationWindow().show("review")
    }

    function startTask() {
        if (taskField.text.trim() === "") {
            return
        }
        const text = taskField.text.trim()
        store.call("task", { instruction: taskField.text, scope: quickScope }, r => {
            if (r) {
                page.remember(text)
                taskField.text = ""
                store.batchID = r.id
                store.path = ""
                applicationWindow().show("board")
            }
        })
    }

    Timer {
        id: debounce
        interval: 400
        onTriggered: page.store.call("estimate", { instruction: taskField.text, scope: page.quickScope }, r => { page.estimate = r })
    }

    // Wide windows: the content keeps a readable width, centred.
    Item {
    implicitHeight: content.implicitHeight
    ColumnLayout {
        id: content
        width: Math.min(parent.width, Kirigami.Units.gridUnit * 80)
        anchors.horizontalCenter: parent.horizontalCenter
        spacing: Kirigami.Units.gridUnit

        GridLayout {
            columns: page.tileColumns
            columnSpacing: Kirigami.Units.largeSpacing
            rowSpacing: Kirigami.Units.largeSpacing
            uniformCellHeights: columns > 1
            Layout.fillWidth: true

            Tile {
                label: i18n("Conforms to rules")
                value: page.stats.conformity !== undefined ? Math.round(page.stats.conformity * 100) + " %" : "–"
                detail: page.stats.hasWeekAgo ? i18n("%1 this week", (page.stats.conformity >= page.stats.weekAgo ? "+" : "") + Math.round((page.stats.conformity - page.stats.weekAgo) * 100)) : i18np("One note checked", "%1 notes checked", page.store.home.notes || 0)
                Sparkline {
                    values: page.stats.spark || []
                    days: page.stats.sparkDays || []
                    Layout.topMargin: Kirigami.Units.smallSpacing
                }
                // The next step that helps most.
                QQC2.Label {
                    visible: page.lever !== null
                    text: page.lever ? i18n("Biggest lever: %1 (%2)", (page.store.home.titles || {})[page.lever.rule] || page.lever.rule, page.lever.count) : ""
                    font: Kirigami.Theme.smallFont
                    color: KanteStyle.mutedTextColor
                    elide: Text.ElideRight
                    Layout.fillWidth: true
                }
            }
            Tile {
                label: i18n("Waiting for you")
                value: page.store.home.waitingHunks || 0
                detail: i18np("change in one batch", "changes in %1 batches", page.store.home.waiting || 0)
                QQC2.Label {
                    visible: page.oldestWaiting !== null
                    text: page.oldestWaiting ? i18n("Oldest: %1", page.store.age(page.oldestWaiting.created)) : ""
                    font: Kirigami.Theme.smallFont
                    color: KanteStyle.mutedTextColor
                }
                QQC2.Label {
                    readonly property int questions: page.waitingBatches.reduce((n, b) => n + b.questions, 0)
                    visible: questions > 0
                    text: i18np("One question from the AI", "%1 questions from the AI", questions)
                    font: Kirigami.Theme.smallFont
                    color: KanteStyle.neutralTextColor
                }
            }
            Tile {
                label: i18n("Streak")
                value: page.stats.streak || 0
                detail: i18np("day in a row", "days in a row", page.stats.streak || 0) + " · " + i18np("one change today", "%1 changes today", page.stats.reviewedToday || 0)
                // Reviewed changes per day this week.
                Row {
                    id: week
                    readonly property var days: page.stats.week || []
                    readonly property int most: Math.max(1, ...days)
                    spacing: 3
                    Layout.topMargin: Kirigami.Units.smallSpacing
                    height: Kirigami.Units.gridUnit * 1.6
                    Repeater {
                        model: week.days
                        delegate: Item {
                            required property int modelData
                            required property int index
                            width: Kirigami.Units.smallSpacing * 2.2
                            height: week.height
                            Rectangle {
                                anchors.bottom: parent.bottom
                                width: parent.width
                                height: parent.modelData > 0 ? Math.max(3, parent.height * parent.modelData / week.most) : 2
                                color: parent.modelData > 0 ? (parent.index === week.days.length - 1 ? KanteStyle.accentColor : Qt.alpha(KanteStyle.positiveTextColor, 0.7)) : KanteStyle.frameColor
                            }
                            HoverHandler { id: dayHover }
                            QQC2.ToolTip.visible: dayHover.hovered
                            QQC2.ToolTip.text: i18np("one change", "%1 changes", parent.modelData)
                        }
                    }
                }
            }
            Tile {
                visible: page.tileColumns === 5
                label: i18n("AI this month")
                value: page.monthUsage.cost > 0 ? page.store.money(page.monthUsage, page.store.currencyOf(null)) : page.store.tokens(page.monthUsage)
                detail: i18np("one task", "%1 tasks", page.monthUsage.tasks) + (page.monthUsage.cost > 0 ? " · " + page.store.tokens(page.monthUsage) : "")
            }
            Tile {
                visible: page.tileColumns === 5
                label: i18n("Last written")
                value: page.lastWrite ? Qt.formatTime(new Date(page.lastWrite.time), Qt.DefaultLocaleShortDate) : "–"
                detail: page.lastWrite ? page.store.fileName(page.lastWrite.path) + " · " + page.store.age(page.lastWrite.time) : i18n("Nothing written yet")
                KanteToolButton {
                    visible: page.lastWrite !== null && !page.lastWrite.undone
                    text: i18n("Undo")
                    icon.name: "edit-undo"
                    onClicked: page.store.call("undoFile", { id: page.lastWrite.batch, path: page.lastWrite.path }, () => {
                        page.store.call("journal", { limit: 1 }, r => { page.lastWrite = r && r.length > 0 ? r[0] : null })
                    })
                }
            }
        }

        // Waiting batches, one click into the review.
        ColumnLayout {
        visible: page.waitingBatches.length > 0
        Layout.fillWidth: true
        spacing: Kirigami.Units.smallSpacing
        Repeater {
            model: page.waitingBatches.slice(0, 3)
            delegate: QQC2.ItemDelegate {
                id: waitRow
                required property var modelData
                Layout.fillWidth: true
                padding: Kirigami.Units.largeSpacing
                background: Surface { selected: waitRow.hovered }
                onClicked: page.openBatch(modelData.id)
                contentItem: RowLayout {
                    spacing: Kirigami.Units.largeSpacing
                    Chip { text: waitRow.modelData.topic || i18n("Task"); tone: KanteStyle.tagColor }
                    QQC2.Label {
                        text: waitRow.modelData.title
                        elide: Text.ElideRight
                        Layout.fillWidth: true
                    }
                    Chip {
                        visible: waitRow.modelData.questions > 0
                        text: i18np("one question", "%1 questions", waitRow.modelData.questions)
                        tone: KanteStyle.neutralTextColor
                    }
                    QQC2.Label {
                        readonly property var c: waitRow.modelData.counts
                        text: i18np("one change open", "%1 changes open", c.hunks - c.accepted - c.rejected)
                        color: KanteStyle.mutedTextColor
                    }
                    Kirigami.Icon {
                        source: "go-next"
                        implicitWidth: Kirigami.Units.iconSizes.small
                        implicitHeight: Kirigami.Units.iconSizes.small
                    }
                }
            }
        }
        QQC2.Label {
            visible: page.waitingBatches.length > 3
            text: i18np("and one more batch in the review line", "and %1 more batches in the review line", page.waitingBatches.length - 3)
            color: KanteStyle.mutedTextColor
        }
        }

        // New task, typed right here.
        QQC2.Control {
            Layout.fillWidth: true
            padding: Kirigami.Units.largeSpacing
            background: Surface {}
            contentItem: ColumnLayout {
                spacing: Kirigami.Units.smallSpacing
                Kirigami.Heading {
                    level: 3
                    text: i18n("New task")
                    font: KanteStyle.headingFont(Kirigami.Theme.defaultFont.pointSize * 1.2)
                }
                RowLayout {
                    spacing: Kirigami.Units.smallSpacing
                    Layout.fillWidth: true
                    QQC2.TextField {
                        id: taskField
                        Layout.fillWidth: true
                        placeholderText: i18n("What should change? The AI finds the notes, you review every change.")
                        onTextChanged: debounce.restart()
                        onAccepted: page.startTask()
                        onActiveFocusChanged: page.store.typing = activeFocus
                        KanteFieldSkin { control: parent }
                    }
                    KanteButton {
                        text: i18n("Start")
                        icon.name: "media-playback-start"
                        emphasis: KanteButton.Emphasis.Primary
                        enabled: taskField.text.trim() !== ""
                        onClicked: page.startTask()
                    }
                    KanteButton {
                        text: i18n("Details…")
                        onClicked: applicationWindow().openTask(taskField.text, page.quickScope.length > 0 ? page.quickScope : null)
                    }
                }
                // The last tasks, one click to use again.
                Flow {
                    visible: page.recentTasks.length > 0 && taskField.text === ""
                    Layout.fillWidth: true
                    spacing: Kirigami.Units.smallSpacing
                    Repeater {
                        model: page.recentTasks
                        delegate: Chip {
                            required property string modelData
                            interactive: true
                            checkable: false
                            plain: true
                            text: modelData
                            onClicked: { taskField.text = modelData; taskField.forceActiveFocus() }
                        }
                    }
                }
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Kirigami.Units.largeSpacing
                    Flow {
                        Layout.fillWidth: true
                        spacing: Kirigami.Units.smallSpacing
                        Repeater {
                            model: page.store.status.allowed || []
                            delegate: Chip {
                                required property string modelData
                                interactive: true
                                text: modelData
                                tone: KanteStyle.tagColor
                                checked: page.quickScope.indexOf(modelData) >= 0
                                onClicked: {
                                    const s = page.quickScope.filter(f => f !== modelData)
                                    if (checked) {
                                        s.push(modelData)
                                    }
                                    page.quickScope = s
                                    debounce.restart()
                                }
                            }
                        }
                    }
                    QQC2.Label {
                        visible: page.estimate !== null && taskField.text !== ""
                        text: page.estimate ? i18np("one note", "%1 notes", page.estimate.notes) + " · " + (page.estimate.hasPrice ? i18n("about %1 %2", page.estimate.cost.toFixed(2), page.estimate.currency || "") : i18n("≈ %1 k tokens at most", Math.round(page.estimate.tokens / 1000))) : ""
                        font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                        color: KanteStyle.mutedTextColor
                    }
                }
            }
        }

        SectionLabel { text: i18n("Findings from checks · no AI") }

        Kirigami.PlaceholderMessage {
            visible: page.findings.length === 0
            Layout.fillWidth: true
            icon.name: "checkmark"
            text: i18n("No findings")
            explanation: i18n("Every note in the allowed folders follows the checks.")
        }

        ColumnLayout {
        Layout.fillWidth: true
        spacing: 0
        Repeater {
            model: page.findings
            delegate: ColumnLayout {
                id: finding
                required property var modelData
                readonly property bool open: page.expanded === modelData.rule
                readonly property string openBatch: (page.store.home.open || {})[modelData.rule] || ""
                Layout.fillWidth: true
                spacing: 0

                QQC2.ItemDelegate {
                    Layout.fillWidth: true
                    implicitHeight: Kirigami.Units.gridUnit * 1.9
                    topPadding: 0
                    bottomPadding: 0
                    highlighted: finding.open
                    onClicked: page.expanded = finding.open ? "" : finding.modelData.rule
                    contentItem: RowLayout {
                        spacing: Kirigami.Units.largeSpacing
                        Kirigami.Icon {
                            source: finding.open ? "go-down" : "go-next"
                            implicitWidth: Kirigami.Units.iconSizes.small
                            implicitHeight: Kirigami.Units.iconSizes.small
                        }
                        QQC2.Label {
                            text: page.store.home.titles ? page.store.home.titles[finding.modelData.rule] : finding.modelData.rule
                            Layout.preferredWidth: Kirigami.Units.gridUnit * 13
                            Layout.fillWidth: page.narrow
                            elide: Text.ElideRight
                        }
                        Item {
                            visible: !page.narrow
                            Layout.preferredWidth: Kirigami.Units.gridUnit * 8
                            Layout.fillWidth: true
                            Layout.maximumWidth: Kirigami.Units.gridUnit * 12
                            implicitHeight: Kirigami.Units.smallSpacing * 1.5
                            Rectangle { anchors.fill: parent; color: KanteStyle.sunkenColor; radius: KanteStyle.active ? 0 : height / 2 }
                            Rectangle {
                                width: parent.width * finding.modelData.count / page.maxCount
                                height: parent.height
                                radius: KanteStyle.active ? 0 : height / 2
                                color: page.store.ruleColor(finding.modelData.rule)
                            }
                        }
                        // What the numbers are: findings, and in how many notes.
                        QQC2.Label {
                            visible: !page.narrow
                            text: page.countText(finding.modelData)
                            color: KanteStyle.mutedTextColor
                            Layout.preferredWidth: Kirigami.Units.gridUnit * 10
                        }
                        KanteToolButton {
                            id: findingAction
                            text: finding.openBatch ? i18n("Open batch") : i18n("Create batch")
                            icon.name: finding.openBatch ? "go-next" : "document-new"
                            display: page.width > Kirigami.Units.gridUnit * 34 ? QQC2.AbstractButton.TextBesideIcon : QQC2.AbstractButton.IconOnly
                            QQC2.ToolTip.visible: hovered && display === QQC2.AbstractButton.IconOnly
                            QQC2.ToolTip.text: text
                            onClicked: {
                                if (finding.openBatch) {
                                    const b = page.store.batch(finding.openBatch)
                                    page.store.batchID = finding.openBatch
                                    page.store.path = ""
                                    applicationWindow().show(b && b.running ? "board" : "review")
                                    return
                                }
                                page.store.call("fromFinding", { rule: finding.modelData.rule }, r => {
                                    if (r) {
                                        page.store.batchID = r.id
                                        page.store.path = ""
                                        applicationWindow().show(r.source === "rules" ? "review" : "board")
                                    }
                                })
                            }
                        }
                        Item { visible: !page.narrow; Layout.fillWidth: true }
                    }
                }
                // Narrow pages: bar and numbers on their own line.
                RowLayout {
                    visible: page.narrow
                    Layout.fillWidth: true
                    Layout.leftMargin: Kirigami.Units.gridUnit * 1.5
                    Layout.bottomMargin: Kirigami.Units.smallSpacing
                    Item {
                        Layout.fillWidth: true
                        implicitHeight: Kirigami.Units.smallSpacing * 1.5
                        Rectangle { anchors.fill: parent; color: KanteStyle.sunkenColor; radius: KanteStyle.active ? 0 : height / 2 }
                        Rectangle {
                            width: parent.width * finding.modelData.count / page.maxCount
                            height: parent.height
                            radius: KanteStyle.active ? 0 : height / 2
                            color: page.store.ruleColor(finding.modelData.rule)
                        }
                    }
                    QQC2.Label {
                        text: page.countText(finding.modelData)
                        color: KanteStyle.mutedTextColor
                        font: Kirigami.Theme.smallFont
                    }
                }

                Loader {
                    active: finding.open
                    visible: active
                    Layout.fillWidth: true
                    Layout.leftMargin: Kirigami.Units.gridUnit * 1.5
                    Layout.bottomMargin: Kirigami.Units.smallSpacing
                    sourceComponent: FindingList { store: page.store; rule: finding.modelData.rule }
                }
            }
        }
        }

        RowLayout {
            Layout.fillWidth: true
            Layout.topMargin: Kirigami.Units.gridUnit
            QQC2.Label {
                Layout.fillWidth: true
                text: [i18n("Vault %1", page.store.status.vaultName || ""), (page.store.status.allowed || []).join(", "),
                       i18np("one note", "%1 notes", page.store.status.notes || 0),
                       page.store.home.checked ? i18n("checked %1", page.store.age(page.store.home.checked)) : ""].filter(s => !!s).join(" · ")
                color: KanteStyle.mutedTextColor
                font: Kirigami.Theme.smallFont
                elide: Text.ElideRight
            }
            KanteToolButton {
                text: i18n("Check again")
                icon.name: "view-refresh"
                onClicked: page.store.call("findings", { refresh: true }, r => {
                    if (r) {
                        page.store.findings = r
                    }
                    page.store.refreshHome()
                })
            }
        }
    }
    }

    component Tile: QQC2.Control {
        id: tile
        property string label
        property var value
        property string detail
        default property alias extra: more.data
        Layout.fillWidth: true
        Layout.fillHeight: true
        padding: Kirigami.Units.largeSpacing
        background: Surface {}
        contentItem: ColumnLayout {
            spacing: Kirigami.Units.smallSpacing
            SectionLabel { text: tile.label }
            QQC2.Label {
                text: tile.value
                font: KanteStyle.monoFont(Kirigami.Theme.defaultFont.pointSize * 2.2, true)
            }
            QQC2.Label {
                text: tile.detail
                color: KanteStyle.mutedTextColor
                wrapMode: Text.Wrap
                Layout.fillWidth: true
            }
            ColumnLayout { id: more }
            Item { Layout.fillHeight: true }
        }
    }
}
