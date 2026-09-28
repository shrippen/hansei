import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante

/**
 * The diff of one file. Every row carries both sides, so in the side-by-side layout old and
 * new scroll as one: a deleted line faces a hatched filler, and wrapped lines keep their
 * partner in the same row. Without wrapping, both sides also scroll sideways together.
 *
 * Decided changes fold to their header, together with the context around them;
 * neighbouring changes with the same decision share one header.
 */
Item {
    id: root

    property var file: null            // FileView from the daemon
    property int currentHunk: 0
    property bool headers: true        // hunk headers with actions
    property var rows: file ? file.rows : []
    property var expanded: ({})        // hunk index → shown although decided
    property var overrides: ({})       // row index → row shown instead (one revealed secret)
    property bool wrap: true
    property real hOffset: 0

    signal decide(string hunk, string decision)
    signal reject(int index)
    signal edit(int index)
    signal feedback(int index)
    signal history(int index)
    signal regenerate(int index)
    signal showAll()
    signal pick(int index)
    signal rule(string ref)
    signal lineFeedback(int hunk, int line, bool old)
    signal revealRow(int row)

    readonly property bool split: !file || file.mode !== "unified"
    readonly property color addColor: KanteStyle.positiveTextColor
    readonly property color delColor: KanteStyle.negativeTextColor
    readonly property font codeFont: KanteStyle.monoFont(Kirigami.Theme.defaultFont.pointSize * 0.92)
    readonly property real numberWidth: Kirigami.Units.gridUnit * 2
    readonly property real signWidth: Kirigami.Units.gridUnit
    // Sideways scrolling: the longest line against the room one side has.
    readonly property int longest: {
        let n = 0
        for (const r of rows) {
            for (const segs of [r.old, r.new]) {
                if (segs) {
                    let len = 0
                    for (const s of segs) {
                        len += s.t.length
                    }
                    n = Math.max(n, len)
                }
            }
        }
        return n
    }
    readonly property real textRoom: (split ? (width - 1) / 2 : width) - numberWidth * (split ? 1 : 2) - signWidth - Kirigami.Units.smallSpacing
    readonly property real hMax: wrap ? 0 : Math.max(0, longest * metrics.averageCharacterWidth - textRoom + Kirigami.Units.gridUnit)
    readonly property var plan: computePlan()

    onWrapChanged: hOffset = 0
    onFileChanged: overrides = {}

    FontMetrics { id: metrics; font: root.codeFont }

    /** Scrolls so the header of hunk i is at the top. */
    function showHunk(i) {
        for (let r = 0; r < rows.length; r++) {
            if (rows[r].kind === "hunk" && rows[r].hunk === i) {
                view.positionViewAtIndex(Math.max(0, r - 2), ListView.Beginning)
                return
            }
        }
    }

    function scrollH(delta) {
        hOffset = Math.max(0, Math.min(hMax, hOffset + delta))
    }

    function hunkState(i) {
        return file && file.hunks[i] ? file.hunks[i].state : "pending"
    }

    function collapsed(i) {
        return headers && i >= 0 && hunkState(i) !== "pending" && !expanded[i] && i !== currentHunk
    }

    /** Per row: hidden, and for a folded header how many neighbours it stands for. */
    function computePlan() {
        const n = rows.length
        const out = new Array(n)
        const prevH = new Array(n)
        const nextH = new Array(n)
        let h = -1
        for (let i = 0; i < n; i++) {
            if (rows[i].kind === "hunk") {
                h = rows[i].hunk
            }
            prevH[i] = h
        }
        h = -1
        for (let i = n - 1; i >= 0; i--) {
            if (rows[i].kind === "hunk") {
                h = rows[i].hunk
            }
            nextH[i] = h
        }
        for (let i = 0; i < n; i++) {
            const r = rows[i]
            let hidden = false
            if (r.kind === "hunk") {
                hidden = !headers
            } else if (r.hunk >= 0) {
                hidden = collapsed(r.hunk)
            } else if (headers) {
                // Context between folded changes (or the file edge) folds with them.
                const p = prevH[i], q = nextH[i]
                hidden = (p >= 0 || q >= 0) && (p < 0 || collapsed(p)) && (q < 0 || collapsed(q))
            }
            out[i] = { hidden: hidden, group: 1 }
        }
        // Neighbouring folded changes with the same decision share the first header.
        let leader = -1
        let lastVisible = -1
        for (let i = 0; i < n; i++) {
            if (out[i].hidden) {
                continue
            }
            const r = rows[i]
            if (r.kind === "hunk" && collapsed(r.hunk)) {
                if (leader >= 0 && lastVisible === leader && hunkState(rows[leader].hunk) === hunkState(r.hunk)) {
                    out[i].hidden = true
                    out[leader].group++
                    continue
                }
                leader = i
            } else {
                leader = -1
            }
            lastVisible = i
        }
        return out
    }

    function escapeHtml(s) {
        return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    }

    /** Rich text of a line; changed words get a stronger background. */
    function html(segs, tone) {
        if (!segs) {
            return ""
        }
        let out = "<span style=\"white-space:" + (wrap ? "pre-wrap" : "pre") + "\">"
        for (const s of segs) {
            const text = escapeHtml(s.t)
            if (s.c) {
                out += "<span style=\"background-color:" + Qt.alpha(tone, 0.35) + ";\">" + text + "</span>"
            } else {
                out += text
            }
        }
        return out + "</span>"
    }

    function masked(segs) {
        return !!segs && segs.some(s => s.t.indexOf("•••") >= 0)
    }

    ListView {
        id: view
        anchors { fill: parent; bottomMargin: hBar.visible ? hBar.height : 0 }
        clip: true
        reuseItems: true
        boundsBehavior: Flickable.StopAtBounds
        model: root.rows
        spacing: 0
        QQC2.ScrollBar.vertical: QQC2.ScrollBar {}

        WheelHandler {
            enabled: !root.wrap
            orientation: Qt.Horizontal
            target: null
            onWheel: event => root.scrollH(-event.angleDelta.x / 2)
        }

        delegate: Loader {
            id: row

            required property var modelData
            required property int index

            readonly property var r: root.overrides[index] || modelData
            readonly property var step: root.plan[index] || { hidden: false, group: 1 }
            width: ListView.view.width
            visible: !step.hidden
            height: visible ? implicitHeight : 0

            sourceComponent: step.hidden ? null
                : r.kind === "hunk" ? headerRow
                : r.kind === "gap" ? gapRow
                : (root.split ? splitRow : unifiedRow)

            Component {
                id: gapRow
                QQC2.ItemDelegate {
                    width: row.width
                    padding: Kirigami.Units.smallSpacing
                    contentItem: QQC2.Label {
                        text: i18np("⋯ one unchanged line", "⋯ %1 unchanged lines", row.r.gap)
                        horizontalAlignment: Text.AlignHCenter
                        color: KanteStyle.mutedTextColor
                        font: Kirigami.Theme.smallFont
                    }
                    onClicked: root.showAll()
                }
            }

            Component {
                id: splitRow
                Item {
                    width: row.width
                    implicitHeight: Math.max(left.implicitHeight, right.implicitHeight)

                    Side {
                        id: left
                        anchors { left: parent.left; top: parent.top; bottom: parent.bottom }
                        width: (parent.width - 1) / 2
                        no: row.r.oldNo || 0
                        segs: row.r.old
                        hunk: row.r.hunk
                        old: true
                        rowIndex: row.index
                        missing: row.r.kind === "add"
                        tone: (row.r.kind === "del" || row.r.kind === "change") ? root.delColor : "transparent"
                        sign: tone.a > 0 ? "−" : ""
                    }
                    Rectangle {
                        x: left.width
                        width: 1
                        height: parent.height
                        color: KanteStyle.ruleColor
                    }
                    Side {
                        id: right
                        anchors { right: parent.right; top: parent.top; bottom: parent.bottom }
                        width: (parent.width - 1) / 2
                        no: row.r.newNo || 0
                        segs: row.r.new
                        hunk: row.r.hunk
                        rowIndex: row.index
                        missing: row.r.kind === "del"
                        tone: (row.r.kind === "add" || row.r.kind === "change") ? root.addColor : "transparent"
                        sign: tone.a > 0 ? "+" : ""
                    }
                }
            }

            Component {
                id: unifiedRow
                Side {
                    width: row.width
                    readonly property bool isOld: row.r.kind === "del"
                    // Two number columns like git: old and new.
                    no: isOld ? row.r.oldNo : (row.r.newNo || 0)
                    oldCol: row.r.kind === "add" ? 0 : (row.r.oldNo || 0)
                    newCol: isOld ? 0 : (row.r.newNo || 0)
                    unified: true
                    old: isOld
                    hunk: row.r.hunk
                    rowIndex: row.index
                    segs: isOld ? row.r.old : row.r.new
                    tone: row.r.kind === "del" ? root.delColor : (row.r.kind === "add" ? root.addColor : "transparent")
                    sign: row.r.kind === "del" ? "−" : (row.r.kind === "add" ? "+" : "")
                }
            }

            Component {
                id: headerRow
                HunkHeader {
                    width: row.width
                    hunk: root.file.hunks[row.r.hunk]
                    total: root.file.hunks.length
                    current: row.r.hunk === root.currentHunk
                    folded: root.collapsed(row.r.hunk)
                    group: row.step.group
                    editable: root.file.status === "open"
                    onDecide: d => root.decide(hunk.id, d)
                    onReject: root.reject(row.r.hunk)
                    onEdit: root.edit(row.r.hunk)
                    onFeedback: root.feedback(row.r.hunk)
                    onHistory: root.history(row.r.hunk)
                    onRegenerate: root.regenerate(row.r.hunk)
                    onRule: ref => root.rule(ref)
                    onToggle: {
                        const e = Object.assign({}, root.expanded)
                        const show = !e[row.r.hunk]
                        for (let k = 0; k < group; k++) {
                            e[row.r.hunk + k] = show
                        }
                        root.expanded = e
                    }
                    onPicked: root.pick(row.r.hunk)
                }
            }
        }
    }

    QQC2.ScrollBar {
        id: hBar
        visible: root.hMax > 0
        orientation: Qt.Horizontal
        anchors { left: parent.left; right: parent.right; bottom: parent.bottom }
        size: root.textRoom / (root.textRoom + root.hMax)
        position: root.hMax > 0 ? root.hOffset / (root.textRoom + root.hMax) : 0
        onPositionChanged: if (pressed) root.hOffset = Math.max(0, Math.min(root.hMax, position * (root.textRoom + root.hMax)))
        policy: QQC2.ScrollBar.AlwaysOn
    }

    /** One side of a row: line number(s), sign and the text. */
    component Side: Item {
        id: side

        property int no: 0
        property bool unified: false    // two number columns, old and new, like git
        property int oldCol: 0
        property int newCol: 0
        property bool old: false
        property int hunk: -1
        property int rowIndex: -1
        property var segs: []
        property bool missing: false
        property color tone: "transparent"
        property string sign: ""
        readonly property bool secret: root.masked(segs)

        implicitHeight: Math.max(text.implicitHeight, number.implicitHeight) + 2

        Rectangle {
            anchors.fill: parent
            color: side.tone.a > 0 ? Qt.alpha(side.tone, 0.12) : "transparent"
        }
        Image {
            anchors.fill: parent
            visible: side.missing
            source: "qrc:/icons/hatch.svg"
            fillMode: Image.Tile
            opacity: 0.5
        }
        QQC2.Label {
            id: otherNumber
            visible: side.unified
            text: side.oldCol > 0 ? side.oldCol : ""
            width: side.unified ? root.numberWidth : 0
            horizontalAlignment: Text.AlignRight
            font: root.codeFont
            color: KanteStyle.mutedTextColor
            anchors { left: parent.left; top: parent.top; topMargin: 1 }
        }
        QQC2.Label {
            id: number
            text: side.unified ? (side.newCol > 0 ? side.newCol : "") : (side.no > 0 ? side.no : "")
            width: root.numberWidth
            horizontalAlignment: Text.AlignRight
            font: root.codeFont
            color: numberHover.hovered ? KanteStyle.accentTextColor : KanteStyle.mutedTextColor
            anchors { left: otherNumber.right; top: parent.top; topMargin: 1 }
            HoverHandler { id: numberHover; enabled: side.no > 0; cursorShape: Qt.PointingHandCursor }
            TapHandler { enabled: side.no > 0; onTapped: root.lineFeedback(side.hunk, side.no, side.old) }
            QQC2.ToolTip.visible: numberHover.hovered
            QQC2.ToolTip.delay: Kirigami.Units.toolTipDelay
            QQC2.ToolTip.text: i18n("Feedback on line %1", side.no)
        }
        QQC2.Label {
            id: signLabel
            text: side.sign
            width: root.signWidth
            horizontalAlignment: Text.AlignHCenter
            font: root.codeFont
            color: side.tone
            anchors { left: number.right; top: parent.top; topMargin: 1 }
        }
        Item {
            id: textBox
            clip: !root.wrap
            anchors { left: signLabel.right; right: parent.right; top: parent.top; bottom: parent.bottom; topMargin: 1; rightMargin: Kirigami.Units.smallSpacing }
            TextEdit {
                id: text
                x: root.wrap ? 0 : -root.hOffset
                width: root.wrap ? textBox.width : Math.max(textBox.width, implicitWidth)
                visible: side.no > 0
                readOnly: true
                selectByMouse: true
                textFormat: TextEdit.RichText
                wrapMode: root.wrap ? TextEdit.WrapAtWordBoundaryOrAnywhere : TextEdit.NoWrap
                text: root.html(side.segs, side.tone.a > 0 ? side.tone : Kirigami.Theme.textColor)
                font: root.codeFont
                color: side.tone.a > 0 ? Kirigami.Theme.textColor : Qt.alpha(Kirigami.Theme.textColor, 0.8)
                selectionColor: Kirigami.Theme.highlightColor
            }
            // Masked secrets: click to see this one value for a few seconds.
            HoverHandler { id: secretHover; enabled: side.secret; cursorShape: Qt.PointingHandCursor }
            TapHandler { enabled: side.secret; onTapped: root.revealRow(side.rowIndex) }
            QQC2.ToolTip.visible: secretHover.hovered
            QQC2.ToolTip.delay: Kirigami.Units.toolTipDelay
            QQC2.ToolTip.text: i18n("Click to show this value for 10 seconds")
        }
    }
}
