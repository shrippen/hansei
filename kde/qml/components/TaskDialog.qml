import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante

/**
 * New task: one sentence, the folders in scope, the provider, and an estimate before
 * anything leaves the machine. "Only find" lists the affected notes without proposing.
 */
Kirigami.Dialog {
    id: dialog

    readonly property var store: applicationWindow().store
    property var scope: []
    property string provider: ""
    property var estimate: null
    property string filter: ""
    property alias findOnly: findSwitch.checked
    // Set before open() to prefill the sentence (start page, Dolphin).
    property string initialText: ""

    readonly property real formWidth: Math.min(Kirigami.Units.gridUnit * 32, applicationWindow().width - Kirigami.Units.gridUnit * 4)
    readonly property var templates: [
        i18n("Replace old host names with the new ones"),
        i18n("Add the missing frontmatter fields"),
        i18n("Move passwords to Vaultwarden and link the entry")
    ]

    title: i18n("New task")
    preferredWidth: formWidth + leftPadding + rightPadding
    // Height from the content (header and footer come on top).
    preferredHeight: form.implicitHeight + topPadding + bottomPadding + Kirigami.Units.gridUnit * 5
    padding: Kirigami.Units.largeSpacing
    standardButtons: Kirigami.Dialog.NoButton

    function openWith(text, folders) {
        initialText = text || ""
        open()
        if (folders) {
            scope = folders
            refreshEstimate()
        }
    }

    onOpened: {
        taskText.text = initialText
        initialText = ""
        scope = []
        filter = ""
        findSwitch.checked = false
        provider = store.defaultProvider ? store.defaultProvider.name : ""
        estimate = null
        taskText.forceActiveFocus()
        taskText.cursorPosition = taskText.length
        refreshEstimate()
    }

    function refreshEstimate() {
        store.call("estimate", { instruction: taskText.text, scope: scope, provider: provider }, r => { estimate = r })
    }

    function toggle(folder) {
        const s = scope.filter(f => f !== folder && !f.startsWith(folder + "/"))
        if (s.length === scope.length) {
            s.push(folder)
        }
        scope = s
        refreshEstimate()
    }

    // A folder is covered when it or a parent is in scope.
    function covered(folder) {
        return scope.some(f => folder === f || folder.startsWith(f + "/"))
    }

    function submit() {
        if (taskText.text.trim() === "") {
            return
        }
        store.call("task", { instruction: taskText.text, scope: scope, provider: provider, findOnly: findSwitch.checked }, r => {
            if (r) {
                store.batchID = r.id
                store.path = ""
                applicationWindow().show("board")
            }
        })
        close()
    }

    Timer { id: debounce; interval: 400; onTriggered: dialog.refreshEstimate() }


    ColumnLayout {
        id: form
        implicitWidth: dialog.formWidth
        spacing: Kirigami.Units.largeSpacing

        QQC2.TextArea {
            id: taskText
            Layout.fillWidth: true
            Layout.preferredHeight: Kirigami.Units.gridUnit * 4
            wrapMode: TextEdit.Wrap
            placeholderText: i18n("What should change? One sentence is enough.")
            onTextChanged: debounce.restart()
            onActiveFocusChanged: dialog.store.typing = activeFocus
        }

        Flow {
            visible: taskText.text === ""
            Layout.fillWidth: true
            spacing: Kirigami.Units.smallSpacing
            Repeater {
                model: dialog.templates
                delegate: Chip {
                    required property string modelData
                    interactive: true
                    checkable: false
                    text: modelData
                    onClicked: { taskText.text = modelData; taskText.forceActiveFocus() }
                }
            }
        }

        RowLayout {
            Layout.fillWidth: true
            SectionLabel { text: i18n("Scope"); Layout.fillWidth: true }
            QQC2.Label {
                text: dialog.scope.length === 0 ? i18n("all allowed folders") : i18np("one folder", "%1 folders", dialog.scope.length)
                color: KanteStyle.mutedTextColor
                font: Kirigami.Theme.smallFont
            }
        }

        // Chosen folders above the tree, removable.
        Flow {
            visible: dialog.scope.length > 0
            Layout.fillWidth: true
            spacing: Kirigami.Units.smallSpacing
            Repeater {
                model: dialog.scope
                delegate: Chip {
                    required property string modelData
                    interactive: true
                    checkable: false
                    text: modelData + "  ✕"
                    tone: KanteStyle.tagColor
                    onClicked: dialog.toggle(modelData)
                }
            }
        }

        Kirigami.SearchField {
            Layout.fillWidth: true
            placeholderText: i18n("Filter folders…")
            onTextChanged: dialog.filter = text.toLowerCase()
            onActiveFocusChanged: dialog.store.typing = activeFocus
        }

        QQC2.ScrollView {
            Layout.fillWidth: true
            Layout.preferredHeight: Math.min(Kirigami.Units.gridUnit * 9, folders.contentHeight + 2)
            QQC2.ScrollBar.horizontal.policy: QQC2.ScrollBar.AlwaysOff
            ListView {
                id: folders
                clip: true
                model: (dialog.store.status.scopes || []).filter(f => dialog.filter === "" || f.toLowerCase().indexOf(dialog.filter) >= 0)
                delegate: QQC2.CheckBox {
                    required property string modelData
                    readonly property int depth: dialog.filter === "" ? modelData.split("/").length - 1 : 0
                    x: Kirigami.Units.gridUnit * depth
                    width: ListView.view.width - x
                    text: dialog.filter === "" ? modelData.split("/").pop() : modelData
                    checked: dialog.covered(modelData)
                    enabled: dialog.scope.indexOf(modelData) >= 0 || !dialog.covered(modelData)
                    onClicked: dialog.toggle(modelData)
                    KanteCheckSkin { control: parent }
                }
            }
        }

        RowLayout {
            Layout.fillWidth: true
            SectionLabel { text: i18n("Provider") }
            QQC2.ComboBox {
                model: dialog.store.providers.map(p => p.name + " · " + p.model)
                currentIndex: Math.max(0, dialog.store.providers.findIndex(p => p.name === dialog.provider))
                onActivated: i => { dialog.provider = dialog.store.providers[i].name; dialog.refreshEstimate() }
                Layout.fillWidth: true
                KanteFieldSkin { control: parent }
            }
        }

        QQC2.Switch {
            id: findSwitch
            text: i18n("Only find notes, propose nothing")
            Layout.fillWidth: true
            KanteCheckSkin { control: parent; shape: KanteCheckSkin.Shape.Switch }
        }

        QQC2.Label {
            visible: dialog.estimate !== null
            text: {
                const e = dialog.estimate
                if (!e) {
                    return ""
                }
                let t = i18np("one note", "%1 notes", e.notes) + " · " + i18n("≈ %1 k tokens at most", Math.round(e.tokens / 1000))
                if (e.hasPrice) {
                    t += " · " + i18n("about %1 %2", e.cost.toFixed(2), e.currency || "")
                }
                if (e.local) {
                    t += " · " + i18n("local model, nothing leaves this computer")
                }
                if (e.blocked && e.blocked.length > 0) {
                    t += " · " + i18np("one note is for local models only", "%1 notes are for local models only", e.blocked.length)
                }
                return t
            }
            font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
            color: KanteStyle.mutedTextColor
            wrapMode: Text.Wrap
            Layout.fillWidth: true
        }
    }

    customFooterActions: [
        Kirigami.Action {
            text: findSwitch.checked ? i18n("Find notes") : i18n("Create batch")
            icon.name: findSwitch.checked ? "search" : "document-new"
            shortcut: "Ctrl+Return"
            enabled: taskText.text.trim() !== ""
            onTriggered: dialog.submit()
        }
    ]

    // After the content: the dialog sizes its first content child.
    KanteScope { target: dialog.contentItem }
}
