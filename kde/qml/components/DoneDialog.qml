import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante

/** The small moment at the end of a batch: what got done, what comes next, undo. */
Kirigami.Dialog {
    id: dialog

    property var store
    property var outcome: null
    readonly property var counts: outcome ? outcome.batch.counts : ({ files: 0, accepted: 0, rejected: 0 })

    signal next()

    readonly property var nextBatch: {
        const id = outcome ? outcome.batch.id : ""
        return store.batches.find(b => b.id !== id && b.column === "review") || null
    }
    property int suggestions: 0
    property var conformityBefore: undefined
    onOpened: {
        // Conformity before and after this batch: the cached value, then a fresh check.
        conformityBefore = (store.home.stats || {}).conformity
        store.call("findings", { refresh: true }, () => store.refreshHome())
        suggestions = 0
        if (outcome) {
            store.call("batch", { id: outcome.batch.id }, r => {
                suggestions = r ? (r.suggestions || []).filter(s => s.status === "open").length : 0
            })
        }
    }

    title: i18n("Batch finished")
    preferredWidth: Math.min(Kirigami.Units.gridUnit * 28, applicationWindow().width - Kirigami.Units.gridUnit * 2)
    // Kirigami.Dialog does not size itself from a layout: the height comes from the content.
    preferredHeight: form.implicitHeight + topPadding + bottomPadding + Kirigami.Units.gridUnit * 5
    padding: Kirigami.Units.gridUnit
    standardButtons: Kirigami.Dialog.NoButton


    ColumnLayout {
        id: form
        spacing: Kirigami.Units.largeSpacing
        Kirigami.Heading {
            text: dialog.outcome ? dialog.outcome.batch.title : ""
            font: KanteStyle.titleFont(Kirigami.Theme.defaultFont.pointSize * 1.5)
            wrapMode: Text.Wrap
            Layout.fillWidth: true
        }
        RowLayout {
            spacing: Kirigami.Units.gridUnit
            Repeater {
                model: [[dialog.counts.files, i18n("files")], [dialog.counts.accepted, i18n("accepted")], [dialog.counts.rejected, i18n("rejected")]]
                delegate: ColumnLayout {
                    required property var modelData
                    QQC2.Label {
                        text: modelData[0]
                        font: KanteStyle.monoFont(Kirigami.Theme.defaultFont.pointSize * 1.8, true)
                    }
                    SectionLabel { text: modelData[1] }
                }
            }
        }
        // Time, tokens, cost and what it did for the rules.
        QQC2.Label {
            Layout.fillWidth: true
            wrapMode: Text.Wrap
            font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
            color: KanteStyle.mutedTextColor
            text: {
                const o = dialog.outcome
                if (!o) {
                    return ""
                }
                const b = o.batch
                const parts = []
                const minutes = Math.max(1, Math.round((new Date(b.updated) - new Date(b.created)) / 60000))
                parts.push(minutes >= 120 ? i18np("one hour", "%1 hours", Math.round(minutes / 60)) : i18np("one minute", "%1 minutes", minutes))
                if (b.usage && b.usage.in + b.usage.out > 0) {
                    parts.push(dialog.store.tokens(b.usage))
                }
                const money = dialog.store.money(b.usage, dialog.store.currencyOf(b))
                if (money) {
                    parts.push(money)
                }
                const now = (dialog.store.home.stats || {}).conformity
                if (now !== undefined) {
                    parts.push(dialog.conformityBefore !== undefined && Math.round(dialog.conformityBefore * 100) !== Math.round(now * 100)
                        ? i18n("Conforms to rules %1 → %2 %", Math.round(dialog.conformityBefore * 100), Math.round(now * 100))
                        : i18n("Conforms to rules: %1 %", Math.round(now * 100)))
                }
                return parts.join(" · ")
            }
        }
        SectionLabel {
            visible: dialog.nextBatch !== null
            text: i18n("Next")
        }
        QQC2.Label {
            visible: dialog.nextBatch !== null
            text: dialog.nextBatch ? dialog.nextBatch.title + " · " + i18np("one file", "%1 files", dialog.nextBatch.counts.files) : ""
            wrapMode: Text.Wrap
            Layout.fillWidth: true
        }
        QQC2.Label {
            visible: dialog.suggestions > 0
            text: i18np("Learned: one rule suggestion from your feedback is waiting.", "Learned: %1 rule suggestions from your feedback are waiting.", dialog.suggestions)
            color: KanteStyle.accentTextColor
            wrapMode: Text.Wrap
            Layout.fillWidth: true
        }
}

    customFooterActions: [
        Kirigami.Action {
            text: i18n("Undo batch")
            icon.name: "edit-undo"
            onTriggered: {
                dialog.store.call("undoBatch", { id: dialog.outcome.batch.id })
                dialog.close()
            }
        },
        Kirigami.Action {
            text: i18n("Next batch")
            icon.name: "go-next"
            onTriggered: {
                dialog.close()
                dialog.next()
            }
        }
    ]

    // After the content: the dialog sizes its first content child.
    KanteDialogSkin { dialog: dialog }
}
