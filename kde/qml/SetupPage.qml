import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Dialogs
import QtQuick.Layouts
import Qt.labs.folderlistmodel
import org.kde.kirigami as Kirigami
import Kante
import "components"

/**
 * First start: choose the vault, then tick the folders the AI may see. Folders that sound
 * private are proposed for the block list.
 */
Kirigami.ScrollablePage {
    id: page

    property string vault: ""
    property var allow: []
    property var block: []

    // Names that usually hold private notes: proposed to block, never allowed by default.
    readonly property var privateHints: ["diary", "tagebuch", "journal", "personen", "people", "therapie", "therapy", "steuer",
                                         "tax", "finanzen", "finance", "gesundheit", "health", "privat", "private", "passw"]

    title: i18n("Welcome to Hansei")
    KantePageTitle { page: page }
    // Kante: the page ground is Kante's ground, not the dialog tint KanteScope hands to the theme.
    background: Rectangle { color: KanteStyle.themed ? KanteStyle.backgroundColor : Kirigami.Theme.backgroundColor }

    function isPrivate(name) {
        const n = name.toLowerCase()
        return privateHints.some(h => n.indexOf(h) >= 0)
    }

    function toggle(list, name, on) {
        const out = list.filter(x => x !== name)
        if (on) {
            out.push(name)
        }
        return out
    }

    // A new vault: suggest IT (or nothing) to allow and the private sounding folders to block.
    function suggest() {
        const names = []
        for (let i = 0; i < folders.count; i++) {
            names.push(folders.get(i, "fileName"))
        }
        allow = names.filter(n => n === "IT")
        block = names.filter(n => isPrivate(n))
    }

    FolderDialog {
        id: picker
        title: i18n("Choose your Obsidian vault")
        onAccepted: page.vault = decodeURIComponent(selectedFolder.toString().replace("file://", ""))
    }

    FolderListModel {
        id: folders
        folder: page.vault ? "file://" + page.vault : ""
        showFiles: false
        showDotAndDotDot: false
        showHidden: false
        sortField: FolderListModel.Name
        onStatusChanged: if (status === FolderListModel.Ready) page.suggest()
    }

    ColumnLayout {
        spacing: Kirigami.Units.largeSpacing

        QQC2.Label {
            text: i18n("Hansei lets an AI improve your notes and shows every change as a diff before anything is written. Nothing leaves the folders you allow.")
            wrapMode: Text.Wrap
            Layout.fillWidth: true
            Layout.maximumWidth: Kirigami.Units.gridUnit * 40
        }

        KanteSectionLabel { text: i18n("Vault") }
        RowLayout {
            Layout.fillWidth: true
            Layout.maximumWidth: Kirigami.Units.gridUnit * 40
            QQC2.TextField {
                text: page.vault
                placeholderText: i18n("/home/you/Obsidian/Vault")
                onEditingFinished: page.vault = text
                Layout.fillWidth: true
                KanteFieldSkin { control: parent }
            }
            KanteButton {
                icon.name: "document-open-folder"
                text: i18n("Choose…")
                onClicked: picker.open()
            }
        }

        KanteSectionLabel {
            visible: folders.count > 0
            text: i18n("Folders")
            Layout.topMargin: Kirigami.Units.largeSpacing
        }
        QQC2.Control {
            visible: folders.count > 0
            Layout.fillWidth: true
            Layout.maximumWidth: Kirigami.Units.gridUnit * 40
            padding: Kirigami.Units.smallSpacing
            background: Surface {}
            contentItem: ColumnLayout {
                spacing: 0
                RowLayout {
                    Layout.fillWidth: true
                    Item { Layout.fillWidth: true }
                    KanteSectionLabel { text: i18n("Allowed"); horizontalAlignment: Text.AlignHCenter; Layout.preferredWidth: Kirigami.Units.gridUnit * 6 }
                    KanteSectionLabel { text: i18n("Blocked"); horizontalAlignment: Text.AlignHCenter; Layout.preferredWidth: Kirigami.Units.gridUnit * 6 }
                }
                Repeater {
                    model: folders
                    delegate: RowLayout {
                        required property string fileName
                        Layout.fillWidth: true
                        QQC2.Label {
                            text: fileName + "/"
                            font: KanteStyle.monoFont(Kirigami.Theme.defaultFont.pointSize)
                            Layout.fillWidth: true
                            leftPadding: Kirigami.Units.smallSpacing
                        }
                        KanteChip {
                            interactive: false
                            visible: page.isPrivate(fileName)
                            text: i18n("sounds private")
                            chipColor: KanteStyle.neutralTextColor
                        }
                        Item {
                            implicitWidth: Kirigami.Units.gridUnit * 6
                            implicitHeight: allowBox.implicitHeight
                            QQC2.CheckBox {
                                id: allowBox
                                anchors.centerIn: parent
                                checked: page.allow.indexOf(fileName) >= 0
                                enabled: page.block.indexOf(fileName) < 0
                                onToggled: page.allow = page.toggle(page.allow, fileName, checked)
                                KanteCheckSkin { control: parent }
                            }
                        }
                        Item {
                            implicitWidth: Kirigami.Units.gridUnit * 6
                            implicitHeight: blockBox.implicitHeight
                            QQC2.CheckBox {
                                id: blockBox
                                anchors.centerIn: parent
                                checked: page.block.indexOf(fileName) >= 0
                                onToggled: {
                                    page.block = page.toggle(page.block, fileName, checked)
                                    if (checked) {
                                        page.allow = page.toggle(page.allow, fileName, false)
                                    }
                                }
                                KanteCheckSkin { control: parent }
                            }
                        }
                    }
                }
            }
        }
        QQC2.Label {
            visible: folders.count > 0
            text: i18n("Blocked folders are never read, not even for the search index. You can change all of this later in the settings.")
            color: KanteStyle.mutedTextColor
            wrapMode: Text.Wrap
            Layout.fillWidth: true
            Layout.maximumWidth: Kirigami.Units.gridUnit * 40
        }

        QQC2.Label {
            visible: Hansei.problem !== ""
            text: Hansei.problem
            color: KanteStyle.negativeTextColor
            wrapMode: Text.Wrap
            Layout.fillWidth: true
        }
        KanteButton {
            text: i18n("Start")
            emphasis: KanteButton.Emphasis.Primary
            enabled: page.vault !== "" && page.allow.length > 0
            onClicked: Hansei.setup(page.vault, page.allow, page.block)
        }
    }
}
