import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante

/** Why reject? Optional reason and quick chips; can also ask the AI for a new proposal. */
Kirigami.Dialog {
    id: dialog

    property int hunkIndex: -1
    property string quick: ""

    signal rejectHunk(int hunkIndex, string reason, bool again, string quick)

    title: i18n("Why reject?")
    padding: Kirigami.Units.largeSpacing
    preferredWidth: Math.min(Kirigami.Units.gridUnit * 28, applicationWindow().width - Kirigami.Units.gridUnit * 2)
    // Kirigami.Dialog does not size itself from a layout: the height comes from the content.
    preferredHeight: form.implicitHeight + topPadding + bottomPadding + Kirigami.Units.gridUnit * 5
    standardButtons: Kirigami.Dialog.NoButton

    readonly property var labels: ({ "wrong-fact": i18n("Wrong fact"), "unneeded": i18n("Not needed"), "too-long": i18n("Too long"), "style": i18n("Style"), "later": i18n("Later") })

    onOpened: {
        reason.text = ""
        quick = ""
        reason.forceActiveFocus()
    }

    // Chip and text combine: "Wrong fact: Borg, not Restic".
    function finish(again) {
        const typed = reason.text.trim()
        const chip = quick ? labels[quick] : ""
        const text = chip && typed ? chip + ": " + typed : (typed || chip)
        rejectHunk(hunkIndex, text, again, quick)
        close()
    }

    ColumnLayout {
        id: form
        spacing: Kirigami.Units.largeSpacing
        QQC2.Label {
            text: i18n("Optional. The reason helps the AI with the next proposal.")
            wrapMode: Text.Wrap
            color: KanteStyle.mutedTextColor
            Layout.fillWidth: true
        }
        Flow {
            spacing: Kirigami.Units.smallSpacing
            Layout.fillWidth: true
            Repeater {
                model: ["wrong-fact", "unneeded", "too-long", "style", "later"]
                delegate: Chip {
                    required property string modelData
                    interactive: true
                    text: dialog.labels[modelData]
                    checked: dialog.quick === modelData
                    onClicked: dialog.quick = dialog.quick === modelData ? "" : modelData
                }
            }
        }
        QQC2.TextField {
            id: reason
            placeholderText: i18n("e.g. Borg, not Restic")
            Layout.fillWidth: true
            Keys.onReturnPressed: event => dialog.finish(!!(event.modifiers & Qt.ControlModifier))
            Keys.onEnterPressed: event => dialog.finish(!!(event.modifiers & Qt.ControlModifier))
            KanteFieldSkin { control: parent }
        }
        QQC2.Label {
            text: i18n("Enter rejects · Ctrl+Enter rejects and asks for a new proposal")
            font: Kirigami.Theme.smallFont
            color: KanteStyle.mutedTextColor
            wrapMode: Text.Wrap
            Layout.fillWidth: true
        }
    }

    customFooterActions: [
        Kirigami.Action {
            text: i18n("Reject and propose again")
            icon.name: "view-refresh"
            onTriggered: dialog.finish(true)
        },
        Kirigami.Action {
            text: i18n("Reject")
            icon.name: "dialog-cancel"
            onTriggered: dialog.finish(false)
        }
    ]

    // After the content: the dialog sizes its first content child.
    KanteDialogSkin { dialog: dialog }
}
