import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante

/**
 * The note as it will read: frontmatter as a properties table like in Obsidian,
 * wikilinks as links, changed paragraphs marked at the margin. Side by side shows
 * before and after with coupled scrolling.
 */
Item {
    id: root

    property var file: null
    property var store
    property bool sideBySide: false

    // Line numbers that changed, per side.
    readonly property var changedNew: marks(false)
    readonly property var changedOld: marks(true)

    function marks(old) {
        const out = {}
        for (const r of (file ? file.rows : [])) {
            if (r.kind === "change" || r.kind === (old ? "del" : "add")) {
                out[old ? r.oldNo : r.newNo] = true
            }
        }
        return out
    }

    /** Splits a note into frontmatter properties and body blocks with their line ranges. */
    function parse(text) {
        const lines = (text || "").split("\n")
        const props = []
        let i = 0
        if (lines[0] === "---") {
            for (i = 1; i < lines.length && lines[i] !== "---"; i++) {
                const m = lines[i].match(/^([^\s:#][^:]*):\s*(.*)$/)
                if (m) {
                    props.push({ key: m[1], value: m[2], start: i + 1, end: i + 1 })
                } else if (props.length > 0 && lines[i].trim() !== "") {
                    const p = props[props.length - 1]
                    p.value += (p.value ? ", " : "") + lines[i].trim().replace(/^-\s*/, "")
                    p.end = i + 1
                }
            }
            i++
        }
        const blocks = []
        let cur = null
        let fence = false
        for (; i < lines.length; i++) {
            const l = lines[i]
            if (l.trim().startsWith("```")) {
                fence = !fence
            }
            if (!fence && l.trim() === "" && !l.trim().startsWith("```")) {
                cur = null
                continue
            }
            // Headings start their own block.
            if (!fence && /^#{1,6}\s/.test(l) && cur) {
                cur = null
            }
            if (!cur) {
                cur = { text: "", start: i + 1, end: i + 1 }
                blocks.push(cur)
            }
            cur.text += (cur.text ? "\n" : "") + l
            cur.end = i + 1
        }
        return { props: props, blocks: blocks }
    }

    function links(md) {
        return md.replace(/!?\[\[([^\]|#]+)(#[^\]|]*)?(\|([^\]]+))?\]\]/g, (all, target, anchor, bar, alias) =>
            "[" + (alias || target + (anchor || "")) + "](" + root.store.obsidianUrl(target.trim()) + ")")
    }

    function touched(changed, start, end) {
        for (let l = start; l <= end; l++) {
            if (changed[l]) {
                return true
            }
        }
        return false
    }

    RowLayout {
        anchors.fill: parent
        spacing: 0

        Note {
            id: before
            visible: root.sideBySide
            Layout.fillWidth: true
            Layout.fillHeight: true
            label: i18n("Before")
            note: root.parse(root.file ? root.file.base : "")
            changed: root.changedOld
            tone: KanteStyle.negativeTextColor
            onContentYChanged: if (!syncing) after.follow(this)
        }
        Kirigami.Separator { visible: root.sideBySide; Layout.fillHeight: true }
        Note {
            id: after
            Layout.fillWidth: true
            Layout.fillHeight: true
            label: root.sideBySide ? i18n("After") : ""
            note: root.parse(root.file ? root.file.content : "")
            changed: root.changedNew
            tone: KanteStyle.positiveTextColor
            onContentYChanged: if (!syncing && root.sideBySide) before.follow(this)
        }
    }

    component Note: Flickable {
        id: note

        property string label
        property var note: ({ props: [], blocks: [] })
        property var changed: ({})
        property color tone
        property bool syncing: false

        // Coupled scrolling: the same share of the note as the other side.
        function follow(other) {
            const range = other.contentHeight - other.height
            syncing = true
            contentY = range > 0 ? other.contentY / range * Math.max(0, contentHeight - height) : 0
            syncing = false
        }

        clip: true
        contentHeight: column.implicitHeight + Kirigami.Units.gridUnit * 2
        boundsBehavior: Flickable.StopAtBounds
        QQC2.ScrollBar.vertical: QQC2.ScrollBar {}

        ColumnLayout {
            id: column
            x: Kirigami.Units.gridUnit
            y: Kirigami.Units.gridUnit
            width: note.width - Kirigami.Units.gridUnit * 2
            spacing: Kirigami.Units.largeSpacing

            SectionLabel { visible: note.label !== ""; text: note.label }

            // Properties like Obsidian shows them.
            QQC2.Control {
                visible: note.note.props.length > 0
                Layout.fillWidth: true
                padding: Kirigami.Units.smallSpacing
                background: Surface { fill: KanteStyle.sunkenColor }
                contentItem: GridLayout {
                    columns: 3
                    columnSpacing: Kirigami.Units.largeSpacing
                    rowSpacing: Kirigami.Units.smallSpacing
                    Repeater {
                        model: note.note.props
                        delegate: Rectangle {
                            required property var modelData
                            required property int index
                            Layout.row: index
                            Layout.column: 0
                            Layout.fillHeight: true
                            implicitWidth: 3
                            color: root.touched(note.changed, modelData.start, modelData.end) ? note.tone : "transparent"
                        }
                    }
                    Repeater {
                        model: note.note.props
                        delegate: QQC2.Label {
                            required property var modelData
                            required property int index
                            Layout.row: index
                            Layout.column: 1
                            text: modelData.key
                            color: KanteStyle.mutedTextColor
                            font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                        }
                    }
                    Repeater {
                        model: note.note.props
                        delegate: QQC2.Label {
                            required property var modelData
                            required property int index
                            Layout.row: index
                            Layout.column: 2
                            Layout.fillWidth: true
                            text: root.links(modelData.value)
                            textFormat: Text.MarkdownText
                            wrapMode: Text.Wrap
                            linkColor: KanteStyle.infoColor
                            onLinkActivated: link => Qt.openUrlExternally(link)
                        }
                    }
                }
            }

            Repeater {
                model: note.note.blocks
                delegate: RowLayout {
                    required property var modelData
                    readonly property bool marked: root.touched(note.changed, modelData.start, modelData.end)
                    Layout.fillWidth: true
                    spacing: Kirigami.Units.largeSpacing
                    Rectangle {
                        Layout.fillHeight: true
                        implicitWidth: 3
                        color: parent.marked ? note.tone : "transparent"
                    }
                    QQC2.Label {
                        Layout.fillWidth: true
                        text: root.links(parent.modelData.text)
                        textFormat: Text.MarkdownText
                        wrapMode: Text.Wrap
                        linkColor: KanteStyle.infoColor
                        onLinkActivated: link => Qt.openUrlExternally(link)
                        HoverHandler { cursorShape: parent.hoveredLink ? Qt.PointingHandCursor : Qt.ArrowCursor }
                    }
                }
            }
        }
    }
}
