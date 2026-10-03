import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import org.kde.syntaxhighlighting
import Kante

/** Edit the new side of one change yourself; saving creates your own version. */
Kirigami.Dialog {
    id: dialog

    property var store
    property string batchID
    property string path
    property var hunk: null
    property int hunkIndex: -1

    signal saved()

    title: hunk ? i18n("Edit change %1", hunkIndex + 1) : i18n("Edit")
    preferredWidth: Math.min(Kirigami.Units.gridUnit * 52, applicationWindow().width - Kirigami.Units.gridUnit * 2)
    // Most of the height goes to the text: the dialog is for writing.
    preferredHeight: Math.min(Kirigami.Units.gridUnit * 34, applicationWindow().height - Kirigami.Units.gridUnit * 2)
    padding: Kirigami.Units.largeSpacing
    standardButtons: Kirigami.Dialog.Save | Kirigami.Dialog.Cancel

    property string before: ""
    readonly property var preview: lineDiff(before, area.text)

    /** Loads the unmasked text of the change: editing needs the real content. */
    function load(batch, file, h, index) {
        batchID = batch
        path = file
        hunk = h
        hunkIndex = index
        store.call("file", { id: batch, path: file, mode: "unified", context: 1, reveal: true }, r => {
            if (!r) {
                return
            }
            const olds = []
            const news = []
            for (const row of r.rows) {
                if (row.hunk === index && row.kind === "add") {
                    news.push((row.new || []).map(s => s.t).join(""))
                } else if (row.hunk === index && row.kind === "del") {
                    olds.push((row.old || []).map(s => s.t).join(""))
                }
            }
            before = olds.join("\n")
            area.text = news.join("\n")
            open()
            area.forceActiveFocus()
        })
    }

    /** Line diff of the vault text against your text, for the live preview (hunks are small). */
    function lineDiff(a, b) {
        const x = a === "" ? [] : a.split("\n")
        const y = b === "" ? [] : b.split("\n")
        const n = x.length, m = y.length
        const lcs = []
        for (let i = 0; i <= n; i++) {
            lcs.push(new Array(m + 1).fill(0))
        }
        for (let i = n - 1; i >= 0; i--) {
            for (let j = m - 1; j >= 0; j--) {
                lcs[i][j] = x[i] === y[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1])
            }
        }
        const out = []
        let i = 0, j = 0
        while (i < n || j < m) {
            if (i < n && j < m && x[i] === y[j]) {
                out.push({ k: " ", t: x[i] }); i++; j++
            } else if (j < m && (i >= n || lcs[i][j + 1] >= lcs[i + 1][j])) {
                out.push({ k: "+", t: y[j] }); j++
            } else {
                out.push({ k: "−", t: x[i] }); i++
            }
        }
        return out
    }

    onAccepted: store.call("editHunk", { id: batchID, path: path, hunk: hunk.id, text: area.text }, () => saved())
    // The unmasked text does not stay in memory longer than needed.
    onClosed: {
        area.text = ""
        before = ""
    }

    ColumnLayout {
        spacing: Kirigami.Units.smallSpacing
        QQC2.Label {
            text: i18n("Your text replaces the proposal for this change and counts as accepted.")
            color: KanteStyle.mutedTextColor
            wrapMode: Text.Wrap
            Layout.fillWidth: true
        }
        RowLayout {
            Layout.fillWidth: true
            // Most of the dialog: header, footer, hint and preview take about fifteen grid units.
            Layout.preferredHeight: Math.max(Kirigami.Units.gridUnit * 6, dialog.preferredHeight - Kirigami.Units.gridUnit * 15)
            spacing: Kirigami.Units.largeSpacing
            ColumnLayout {
                Layout.fillWidth: true
                Layout.fillHeight: true
                Layout.preferredWidth: 1
                KanteSectionLabel { text: i18n("In the vault") }
                QQC2.ScrollView {
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    QQC2.TextArea {
                        KanteFieldSkin { control: parent }
                        readOnly: true
                        text: dialog.before
                        placeholderText: i18n("(new lines)")
                        font: KanteStyle.monoFont(Kirigami.Theme.defaultFont.pointSize)
                        wrapMode: TextEdit.Wrap
                        color: KanteStyle.mutedTextColor
                    }
                }
            }
            ColumnLayout {
                Layout.fillWidth: true
                Layout.fillHeight: true
                Layout.preferredWidth: 1
                KanteSectionLabel { text: i18n("Your text") }
                QQC2.ScrollView {
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    QQC2.TextArea {
                        KanteFieldSkin { control: parent }
                        id: area
                        font: KanteStyle.monoFont(Kirigami.Theme.defaultFont.pointSize)
                        wrapMode: TextEdit.Wrap
                        onActiveFocusChanged: dialog.store.typing = activeFocus
                        SyntaxHighlighter {
                            textEdit: area
                            definition: "Markdown"
                        }
                    }
                }
            }
        }
        KanteSectionLabel { text: i18n("Change against the vault") }
        QQC2.ScrollView {
            Layout.fillWidth: true
            Layout.preferredHeight: Kirigami.Units.gridUnit * 7
            ListView {
                model: dialog.preview
                clip: true
                delegate: QQC2.Label {
                    required property var modelData
                    width: ListView.view.width
                    text: modelData.k + " " + modelData.t
                    font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                    elide: Text.ElideRight
                    color: modelData.k === "+" ? KanteStyle.positiveTextColor : modelData.k === "−" ? KanteStyle.negativeTextColor : KanteStyle.mutedTextColor
                    background: Rectangle {
                        color: modelData.k === "+" ? Qt.alpha(KanteStyle.positiveTextColor, 0.1) : modelData.k === "−" ? Qt.alpha(KanteStyle.negativeTextColor, 0.1) : "transparent"
                    }
                }
            }
        }
    }

    // After the content: the dialog sizes its first content child.
    KanteDialogSkin { dialog: dialog }
}
