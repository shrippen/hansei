import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante

/** All keys of the review line at a glance (the ? key). */
Kirigami.Dialog {
    id: dialog

    title: i18n("Keys")
    padding: Kirigami.Units.largeSpacing
    preferredWidth: Math.min(Kirigami.Units.gridUnit * 34, applicationWindow().width - Kirigami.Units.gridUnit * 2)
    preferredHeight: Math.min(grid.implicitHeight + Kirigami.Units.gridUnit * 6, applicationWindow().height - Kirigami.Units.gridUnit * 2)
    standardButtons: Kirigami.Dialog.Close

    readonly property var groups: [
        [i18n("Decide"), [["A", i18n("Accept the change")], ["R", i18n("Reject the change")], ["E", i18n("Edit the change")],
                          ["Shift+A", i18n("Accept every open change of the file")], ["Shift+X", i18n("Reject every open change of the file")],
                          ["U", i18n("Undo the written file")]]],
        [i18n("Move"), [["J / ↓", i18n("Next change")], ["K / ↑", i18n("Previous change")],
                        ["Shift+J", i18n("Next file")], ["Shift+K", i18n("Previous file")]]],
        [i18n("Talk to the AI"), [["F", i18n("Feedback on the change")], ["Shift+F", i18n("Feedback on the batch")],
                                  ["Shift+R", i18n("Propose again")]]],
        [i18n("View"), [["M", i18n("Side by side, unified, rendered")], ["V", i18n("Previous version")], ["Shift+V", i18n("History of the change")],
                        ["?", i18n("This overview")]]],
        [i18n("Everywhere"), [["N", i18n("New task")], ["S", i18n("Start")], ["W", i18n("Workbench")], ["O", i18n("Journal")], ["Ctrl+,", i18n("Settings")]]]
    ]


    GridLayout {
        id: grid
        columns: 2
        columnSpacing: Kirigami.Units.gridUnit
        rowSpacing: Kirigami.Units.smallSpacing

        Repeater {
            model: dialog.groups
            delegate: ColumnLayout {
                required property var modelData
                Layout.alignment: Qt.AlignTop
                Layout.fillWidth: true
                Layout.preferredWidth: Kirigami.Units.gridUnit * 15
                spacing: Kirigami.Units.smallSpacing
                SectionLabel { text: modelData[0]; Layout.topMargin: Kirigami.Units.smallSpacing }
                Repeater {
                    model: modelData[1]
                    delegate: RowLayout {
                        required property var modelData
                        spacing: Kirigami.Units.largeSpacing
                        QQC2.Label {
                            text: modelData[0].indexOf("+") > 0 ? Hansei.keyName(modelData[0]) : modelData[0]
                            font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize, true)
                            Layout.preferredWidth: Kirigami.Units.gridUnit * 4
                        }
                        QQC2.Label {
                            text: modelData[1]
                            Layout.fillWidth: true
                            wrapMode: Text.Wrap
                        }
                    }
                }
            }
        }
    }

    // After the content: the dialog sizes its first content child.
    KanteDialogSkin { dialog: dialog }
}
