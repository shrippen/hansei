import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Dialogs
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante
import "components"

/** Folders (allowed, local only, blocked) with their rulebooks, the checks, AI providers, and the style. */
Kirigami.ScrollablePage {
    id: page

    readonly property var store: applicationWindow().store
    property var settings: ({ folders: [], providers: [], allow: [], block: [], localOnly: [], codenames: {}, required: {}, rulebooks: [] })
    property var opened: ({})        // folder path → sub folders shown
    property var checks: ({})        // provider name → result of "Test connection"
    property string rulebookFolder: ""

    readonly property real columnWidth: Kirigami.Units.gridUnit * 7.5
    // Form labels inside delegates: attached properties there do not see i18n.
    readonly property string labelModel: i18n("Model:")
    readonly property string labelResult: i18n("Result:")
    readonly property string labelKey: i18n("Key:")
    readonly property string labelPrice: i18n("Price per million tokens:")
    readonly property string labelFallback: i18n("Fallback:")

    title: i18n("Settings")
    KantePageTitle { page: page }

    function load() {
        store.call("settings", {}, r => { if (r) settings = r })
    }
    function patch(p) {
        store.call("setSettings", p, r => { if (r) settings = r })
    }
    function toggled(list, name, on) {
        const out = (list || []).filter(x => x !== name)
        if (on) {
            out.push(name)
        }
        return out
    }

    // A folder is shown when all its parents are opened.
    function shown(f) {
        const parts = f.path.split("/")
        for (let i = 1; i < parts.length; i++) {
            if (!opened[parts.slice(0, i).join("/")]) {
                return false
            }
        }
        return true
    }
    function hasChildren(f) {
        return settings.folders.some(o => o.path.startsWith(f.path + "/"))
    }
    function own(list, path) {
        return (list || []).indexOf(path) >= 0
    }

    function rulebookOf(folder) {
        const rb = (settings.rulebooks || []).find(r => r.folder === folder)
        return rb ? rb.files : []
    }
    function setRulebook(folder, files) {
        const rbs = (settings.rulebooks || []).filter(r => r.folder !== folder)
        if (files.length > 0) {
            rbs.push({ folder: folder, files: files })
        }
        patch({ rulebooks: rbs })
    }

    function updateProvider(index, change) {
        const list = settings.providers.map((p, i) => i === index ? Object.assign({}, p, change) : p)
        patch({ providers: list })
    }

    Component.onCompleted: load()
    Connections {
        target: page.store
        function onSettingsChanged() { page.load() }
    }

    FileDialog {
        id: notePicker
        title: i18n("Choose a rulebook note")
        nameFilters: [i18n("Notes (*.md)")]
        currentFolder: "file://" + page.settings.vault + "/" + page.rulebookFolder
        onAccepted: {
            const full = decodeURIComponent(selectedFile.toString().replace("file://", ""))
            if (!full.startsWith(page.settings.vault + "/")) {
                applicationWindow().toast(i18n("The rulebook has to be a note in the vault."))
                return
            }
            const rel = full.slice(page.settings.vault.length + 1)
            const files = page.rulebookOf(page.rulebookFolder)
            if (files.indexOf(rel) < 0) {
                page.setRulebook(page.rulebookFolder, files.concat([rel]))
            }
        }
    }

    ColumnLayout {
        spacing: Kirigami.Units.gridUnit

        // ---------- Folders ----------
        SectionLabel { text: i18n("Vault and folders") }
        QQC2.Label {
            Layout.fillWidth: true
            wrapMode: Text.Wrap
            text: i18n("The AI only sees allowed folders; sub folders can be allowed on their own. Blocked wins over everything: never read, not even for the index. Local only is never sent to external providers.")
            color: KanteStyle.mutedTextColor
        }

        QQC2.Control {
            Layout.fillWidth: true
            padding: Kirigami.Units.smallSpacing
            background: Surface {}
            contentItem: ColumnLayout {
                spacing: 0

                // Column heads once, the check boxes below.
                RowLayout {
                    Layout.fillWidth: true
                    Layout.bottomMargin: Kirigami.Units.smallSpacing
                    QQC2.Label {
                        text: page.settings.vault || ""
                        font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                        color: KanteStyle.mutedTextColor
                        elide: Text.ElideMiddle
                        Layout.fillWidth: true
                        leftPadding: Kirigami.Units.smallSpacing
                    }
                    Repeater {
                        model: [i18n("Allowed"), i18n("Local only"), i18n("Blocked")]
                        delegate: SectionLabel {
                            required property string modelData
                            text: modelData
                            horizontalAlignment: Text.AlignHCenter
                            Layout.preferredWidth: page.columnWidth
                        }
                    }
                }
                Kirigami.Separator { Layout.fillWidth: true }

                Repeater {
                    model: page.settings.folders
                    delegate: ColumnLayout {
                        id: folder
                        required property var modelData
                        readonly property bool ownAllow: page.own(page.settings.allow, modelData.path)
                        readonly property bool ownLocal: page.own(page.settings.localOnly, modelData.path)
                        readonly property bool ownBlock: page.own(page.settings.block, modelData.path)
                        visible: page.shown(modelData)
                        Layout.fillWidth: true
                        spacing: 0

                        RowLayout {
                            Layout.fillWidth: true
                            Layout.leftMargin: Kirigami.Units.gridUnit * 1.2 * folder.modelData.depth
                            QQC2.ToolButton {
                                icon.name: page.opened[folder.modelData.path] ? "go-down" : "go-next"
                                opacity: page.hasChildren(folder.modelData) ? 1 : 0
                                enabled: opacity > 0
                                onClicked: {
                                    const o = Object.assign({}, page.opened)
                                    o[folder.modelData.path] = !o[folder.modelData.path]
                                    page.opened = o
                                }
                            }
                            QQC2.Label {
                                text: folder.modelData.name + "/"
                                font: KanteStyle.monoFont(Kirigami.Theme.defaultFont.pointSize, folder.modelData.depth === 0)
                                color: folder.modelData.blocked ? KanteStyle.negativeTextColor : folder.modelData.allowed ? KanteStyle.textColor : KanteStyle.mutedTextColor
                                elide: Text.ElideRight
                                Layout.fillWidth: true
                            }
                            // Inherited states show as checked but can only change at the folder that sets them.
                            Cell {
                                checked: folder.modelData.allowed
                                enabled: !folder.modelData.blocked && (folder.ownAllow || !folder.modelData.allowed)
                                onToggled: page.patch({ allow: page.toggled(page.settings.allow, folder.modelData.path, checked) })
                                tip: folder.modelData.allowed && !folder.ownAllow ? i18n("Allowed by a parent folder") : ""
                            }
                            Cell {
                                checked: folder.modelData.localOnly
                                enabled: folder.modelData.allowed && (folder.ownLocal || !folder.modelData.localOnly)
                                onToggled: page.patch({ localOnly: page.toggled(page.settings.localOnly, folder.modelData.path, checked) })
                                tip: folder.modelData.localOnly && !folder.ownLocal ? i18n("Local only through a parent folder") : ""
                            }
                            Cell {
                                checked: folder.modelData.blocked
                                enabled: folder.ownBlock || !folder.modelData.blocked
                                onToggled: page.patch({ block: page.toggled(page.settings.block, folder.modelData.path, checked) })
                                tip: folder.modelData.blocked && !folder.ownBlock ? i18n("Blocked by a parent folder") : ""
                            }
                        }

                        // Rulebook of an allowed folder that is allowed on its own.
                        Flow {
                            visible: folder.ownAllow
                            Layout.fillWidth: true
                            Layout.leftMargin: Kirigami.Units.gridUnit * (1.2 * folder.modelData.depth + 2.2)
                            Layout.bottomMargin: Kirigami.Units.smallSpacing
                            spacing: Kirigami.Units.smallSpacing
                            QQC2.Label {
                                text: i18n("Rulebook:")
                                color: KanteStyle.mutedTextColor
                                font: Kirigami.Theme.smallFont
                                height: Kirigami.Units.gridUnit * 1.4
                                verticalAlignment: Text.AlignVCenter
                            }
                            Repeater {
                                model: page.rulebookOf(folder.modelData.path)
                                delegate: Chip {
                                    required property string modelData
                                    plain: true
                                    interactive: true
                                    checkable: false
                                    text: modelData + "  ✕"
                                    tone: KanteStyle.infoColor
                                    onClicked: page.setRulebook(folder.modelData.path, page.rulebookOf(folder.modelData.path).filter(f => f !== modelData))
                                    QQC2.ToolTip.visible: hovered
                                    QQC2.ToolTip.text: i18n("Remove from the rulebook")
                                }
                            }
                            QQC2.Label {
                                visible: page.rulebookOf(folder.modelData.path).length === 0
                                text: (folder.modelData.rulebook || []).length > 0 ? i18n("default: %1", folder.modelData.rulebook.join(", ")) : i18n("none")
                                color: KanteStyle.mutedTextColor
                                font: Kirigami.Theme.smallFont
                                height: Kirigami.Units.gridUnit * 1.4
                                verticalAlignment: Text.AlignVCenter
                            }
                            QQC2.ToolButton {
                                text: i18n("Add note…")
                                icon.name: "list-add"
                                height: Kirigami.Units.gridUnit * 1.4
                                onClicked: {
                                    page.rulebookFolder = folder.modelData.path
                                    notePicker.open()
                                }
                            }
                        }
                    }
                }
            }
        }

        // ---------- Checks ----------
        SectionLabel { text: i18n("Checks"); Layout.topMargin: Kirigami.Units.gridUnit }
        QQC2.Label {
            Layout.fillWidth: true
            wrapMode: Text.Wrap
            color: KanteStyle.mutedTextColor
            text: i18n("The checks run without AI and fill the findings on the start page.")
        }

        QQC2.Control {
            Layout.fillWidth: true
            padding: Kirigami.Units.largeSpacing
            background: Surface {}
            contentItem: ColumnLayout {
                spacing: Kirigami.Units.smallSpacing
                RowLayout {
                    SectionLabel { text: i18n("Old code names"); Layout.fillWidth: true }
                    SectionLabel { text: i18n("Replace with"); Layout.preferredWidth: Kirigami.Units.gridUnit * 14 }
                    Item { implicitWidth: Kirigami.Units.iconSizes.medium }
                }
                Repeater {
                    model: Object.keys(page.settings.codenames || {}).sort()
                    delegate: RowLayout {
                        required property string modelData
                        QQC2.Label {
                            text: modelData
                            font: KanteStyle.monoFont(Kirigami.Theme.defaultFont.pointSize)
                            Layout.fillWidth: true
                        }
                        QQC2.TextField {
                            text: page.settings.codenames[modelData] || ""
                            placeholderText: i18n("only report")
                            Layout.preferredWidth: Kirigami.Units.gridUnit * 14
                            onEditingFinished: if (text !== (page.settings.codenames[modelData] || "")) {
                                const c = Object.assign({}, page.settings.codenames)
                                c[modelData] = text.trim()
                                page.patch({ codenames: c })
                            }
                            onActiveFocusChanged: page.store.typing = activeFocus
                            KanteFieldSkin { control: parent }
                        }
                        QQC2.ToolButton {
                            icon.name: "edit-delete"
                            onClicked: {
                                const c = Object.assign({}, page.settings.codenames)
                                delete c[modelData]
                                page.patch({ codenames: c })
                            }
                            QQC2.ToolTip.visible: hovered
                            QQC2.ToolTip.text: i18n("Remove")
                        }
                    }
                }
                RowLayout {
                    QQC2.TextField {
                        id: newOld
                        placeholderText: i18n("Old name")
                        Layout.fillWidth: true
                        onActiveFocusChanged: page.store.typing = activeFocus
                        KanteFieldSkin { control: parent }
                    }
                    QQC2.TextField {
                        id: newNew
                        placeholderText: i18n("New name (empty: only report)")
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 14
                        onActiveFocusChanged: page.store.typing = activeFocus
                        onAccepted: addName.clicked()
                        KanteFieldSkin { control: parent }
                    }
                    QQC2.ToolButton {
                        id: addName
                        icon.name: "list-add"
                        enabled: newOld.text.trim() !== ""
                        onClicked: {
                            const c = Object.assign({}, page.settings.codenames)
                            c[newOld.text.trim()] = newNew.text.trim()
                            page.patch({ codenames: c })
                            newOld.text = ""
                            newNew.text = ""
                        }
                        QQC2.ToolTip.visible: hovered
                        QQC2.ToolTip.text: i18n("Add")
                    }
                }

                Kirigami.Separator { Layout.fillWidth: true; Layout.topMargin: Kirigami.Units.largeSpacing; Layout.bottomMargin: Kirigami.Units.largeSpacing }

                RowLayout {
                    SectionLabel { text: i18n("Folder"); Layout.fillWidth: true }
                    SectionLabel { text: i18n("Required frontmatter fields"); Layout.preferredWidth: Kirigami.Units.gridUnit * 14 }
                    Item { implicitWidth: Kirigami.Units.iconSizes.medium }
                }
                Repeater {
                    model: Object.keys(page.settings.required || {}).sort()
                    delegate: RowLayout {
                        required property string modelData
                        QQC2.Label {
                            text: modelData + "/"
                            font: KanteStyle.monoFont(Kirigami.Theme.defaultFont.pointSize)
                            Layout.fillWidth: true
                        }
                        QQC2.TextField {
                            text: (page.settings.required[modelData] || []).join(", ")
                            Layout.preferredWidth: Kirigami.Units.gridUnit * 14
                            onEditingFinished: {
                                const r = Object.assign({}, page.settings.required)
                                r[modelData] = text.split(",").map(s => s.trim()).filter(s => s !== "")
                                page.patch({ required: r })
                            }
                            onActiveFocusChanged: page.store.typing = activeFocus
                            KanteFieldSkin { control: parent }
                        }
                        QQC2.ToolButton {
                            icon.name: "edit-delete"
                            onClicked: {
                                const r = Object.assign({}, page.settings.required)
                                delete r[modelData]
                                page.patch({ required: r })
                            }
                            QQC2.ToolTip.visible: hovered
                            QQC2.ToolTip.text: i18n("Remove")
                        }
                    }
                }
                RowLayout {
                    QQC2.ComboBox {
                        id: reqFolder
                        Layout.fillWidth: true
                        model: page.settings.folders.filter(f => f.allowed).map(f => f.path)
                        KanteFieldSkin { control: parent }
                    }
                    QQC2.TextField {
                        id: reqFields
                        placeholderText: i18n("e.g. Gerät, Zuständig")
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 14
                        onActiveFocusChanged: page.store.typing = activeFocus
                        onAccepted: addReq.clicked()
                        KanteFieldSkin { control: parent }
                    }
                    QQC2.ToolButton {
                        id: addReq
                        icon.name: "list-add"
                        enabled: reqFolder.currentText !== "" && reqFields.text.trim() !== ""
                        onClicked: {
                            const r = Object.assign({}, page.settings.required)
                            r[reqFolder.currentText] = reqFields.text.split(",").map(s => s.trim()).filter(s => s !== "")
                            page.patch({ required: r })
                            reqFields.text = ""
                        }
                        QQC2.ToolTip.visible: hovered
                        QQC2.ToolTip.text: i18n("Add")
                    }
                }

                Kirigami.Separator { Layout.fillWidth: true; Layout.topMargin: Kirigami.Units.largeSpacing; Layout.bottomMargin: Kirigami.Units.largeSpacing }

                RowLayout {
                    QQC2.Label {
                        text: i18n("Review date older than")
                        Layout.fillWidth: true
                    }
                    QQC2.SpinBox {
                        from: 0
                        to: 3650
                        stepSize: 30
                        value: page.settings.reviewDays || 0
                        editable: true
                        onValueModified: page.patch({ reviewDays: value })
                    }
                    QQC2.Label { text: i18n("days (0: off)") }
                }
            }
        }

        // ---------- Providers ----------
        SectionLabel { text: i18n("AI providers"); Layout.topMargin: Kirigami.Units.gridUnit }
        QQC2.ButtonGroup { id: defaults }
        Repeater {
            model: page.settings.providers
            delegate: QQC2.Control {
                id: prov
                required property var modelData
                required property int index
                readonly property var check: page.checks[modelData.name] || null
                Layout.fillWidth: true
                padding: Kirigami.Units.largeSpacing
                background: Surface { selected: prov.modelData.default }
                contentItem: ColumnLayout {
                    spacing: Kirigami.Units.smallSpacing
                    RowLayout {
                        QQC2.RadioButton {
                            text: prov.modelData.name
                            checked: prov.modelData.default
                            QQC2.ButtonGroup.group: defaults
                            onToggled: if (checked) page.patch({ provider: prov.modelData.name })
                            KanteCheckSkin { control: parent; shape: KanteCheckSkin.Shape.Radio }
                        }
                        QQC2.Label {
                            text: prov.modelData.baseUrl || ""
                            font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
                            color: KanteStyle.mutedTextColor
                            elide: Text.ElideRight
                            Layout.fillWidth: true
                        }
                        Chip { visible: prov.modelData.local; text: i18n("local"); tone: KanteStyle.neutralTextColor }
                        Chip {
                            visible: prov.modelData.kind !== "demo"
                            text: prov.modelData.hasKey ? i18n("key stored") : i18n("no key")
                            tone: prov.modelData.hasKey ? KanteStyle.positiveTextColor : KanteStyle.mutedTextColor
                        }
                        QQC2.ToolButton {
                            icon.name: "edit-delete"
                            text: i18n("Remove provider")
                            display: QQC2.AbstractButton.IconOnly
                            enabled: !prov.modelData.default
                            onClicked: page.patch({ providers: page.settings.providers.filter((_, i) => i !== prov.index) })
                            QQC2.ToolTip.text: text
                            QQC2.ToolTip.visible: hovered
                        }
                    }
                    // A plain grid: Kirigami.FormLayout inside a Repeater delegate loses the delegate scope.
                    GridLayout {
                        Layout.fillWidth: true
                        columns: 2
                        columnSpacing: Kirigami.Units.largeSpacing
                        QQC2.Label {
                            text: page.labelModel
                            Layout.alignment: Qt.AlignRight | Qt.AlignVCenter
                        }
                        RowLayout {
                            Layout.fillWidth: true
                            QQC2.ComboBox {
                                id: modelBox
                                editable: true
                                Layout.fillWidth: true
                                model: prov.check && prov.check.models.length > 0 ? prov.check.models : [prov.modelData.model]
                                Component.onCompleted: editText = prov.modelData.model
                                onModelChanged: currentIndex = Math.max(0, find(prov.modelData.model))
                                onAccepted: if (editText !== prov.modelData.model) page.updateProvider(prov.index, { model: editText })
                                onActivated: i => page.updateProvider(prov.index, { model: textAt(i) })
                                KanteFieldSkin { control: parent }
                            }
                            QQC2.Button {
                                text: i18n("Test connection")
                                icon.name: "network-connect"
                                onClicked: {
                                    const c = Object.assign({}, page.checks)
                                    c[prov.modelData.name] = { busy: true, models: [] }
                                    page.checks = c
                                    page.store.call("checkProvider", { name: prov.modelData.name }, r => {
                                        const d = Object.assign({}, page.checks)
                                        d[prov.modelData.name] = r || { ok: false, message: "", models: [] }
                                        page.checks = d
                                    })
                                }
                            }
                        }
                        QQC2.Label {
                            visible: prov.check !== null
                            text: page.labelResult
                            Layout.alignment: Qt.AlignRight | Qt.AlignVCenter
                        }
                        QQC2.Label {
                            visible: prov.check !== null
                            Layout.fillWidth: true
                            text: !prov.check ? "" : prov.check.busy ? i18n("Testing…")
                                : prov.check.ok ? i18n("Works · %1", prov.check.models.length > 0 ? i18np("one model", "%1 models", prov.check.models.length) : i18n("answer: %1", prov.check.message))
                                : prov.check.message
                            color: !prov.check || prov.check.busy ? KanteStyle.mutedTextColor : prov.check.ok ? KanteStyle.positiveTextColor : KanteStyle.negativeTextColor
                            wrapMode: Text.Wrap
                        }
                        QQC2.Label {
                            visible: prov.modelData.kind !== "demo"
                            text: page.labelKey
                            Layout.alignment: Qt.AlignRight | Qt.AlignVCenter
                        }
                        RowLayout {
                            visible: prov.modelData.kind !== "demo"
                            Layout.fillWidth: true
                            QQC2.TextField {
                                id: key
                                echoMode: TextInput.Password
                                placeholderText: prov.modelData.kind === "anthropic" ? i18n("API key (stored in KWallet)") : i18n("API key, if the endpoint needs one")
                                Layout.fillWidth: true
                                onActiveFocusChanged: page.store.typing = activeFocus
                                KanteFieldSkin { control: parent }
                            }
                            QQC2.Button {
                                text: i18n("Save key")
                                enabled: key.text !== ""
                                onClicked: page.store.call("setKey", { provider: prov.modelData.name, key: key.text }, () => { key.text = ""; page.load() })
                            }
                        }
                        QQC2.Label {
                            text: page.labelPrice
                            Layout.alignment: Qt.AlignRight | Qt.AlignVCenter
                        }
                        RowLayout {
                            Layout.fillWidth: true
                            QQC2.TextField {
                                text: prov.modelData.priceIn ? prov.modelData.priceIn : ""
                                placeholderText: i18n("in")
                                validator: DoubleValidator { bottom: 0 }
                                Layout.preferredWidth: Kirigami.Units.gridUnit * 5
                                onEditingFinished: page.updateProvider(prov.index, { priceIn: Number(text.replace(",", ".")) || 0 })
                                KanteFieldSkin { control: parent }
                            }
                            QQC2.TextField {
                                text: prov.modelData.priceOut ? prov.modelData.priceOut : ""
                                placeholderText: i18n("out")
                                validator: DoubleValidator { bottom: 0 }
                                Layout.preferredWidth: Kirigami.Units.gridUnit * 5
                                onEditingFinished: page.updateProvider(prov.index, { priceOut: Number(text.replace(",", ".")) || 0 })
                                KanteFieldSkin { control: parent }
                            }
                            QQC2.ComboBox {
                                model: ["USD", "EUR"]
                                currentIndex: prov.modelData.currency === "EUR" ? 1 : 0
                                onActivated: i => page.updateProvider(prov.index, { currency: textAt(i) })
                                KanteFieldSkin { control: parent }
                            }
                        }
                        QQC2.Label {
                            visible: prov.modelData.kind === "anthropic"
                            text: page.labelFallback
                            Layout.alignment: Qt.AlignRight | Qt.AlignVCenter
                        }
                        QQC2.Switch {
                            visible: prov.modelData.kind === "anthropic"
                            Layout.fillWidth: true
                            text: i18n("Switch to a similar model when this one is overloaded")
                            checked: !!prov.modelData.fallback
                            onToggled: page.updateProvider(prov.index, { fallback: checked ? "default" : "" })
                            KanteCheckSkin { control: parent; shape: KanteCheckSkin.Shape.Switch }
                        }
                    }
                }
            }
        }

        // New provider.
        QQC2.Control {
            Layout.fillWidth: true
            padding: Kirigami.Units.largeSpacing
            background: Surface {}
            contentItem: Kirigami.FormLayout {
                QQC2.ComboBox {
                    id: kind
                    Kirigami.FormData.label: i18n("Kind:")
                    model: [i18n("Claude (Anthropic)"), i18n("OpenAI-compatible (Ollama, LM Studio, …)")]
                    KanteFieldSkin { control: parent }
                }
                QQC2.TextField { id: pname; Kirigami.FormData.label: i18n("Name:"); placeholderText: kind.currentIndex === 0 ? "claude" : "ollama"; KanteFieldSkin { control: parent } }
                QQC2.TextField { id: pmodel; Kirigami.FormData.label: i18n("Model:"); placeholderText: kind.currentIndex === 0 ? "claude-opus-5" : "qwen3"; KanteFieldSkin { control: parent } }
                QQC2.TextField { id: purl; visible: kind.currentIndex === 1; Kirigami.FormData.label: i18n("Address:"); placeholderText: "http://localhost:11434/v1"; KanteFieldSkin { control: parent } }
                QQC2.Button {
                    text: i18n("Add provider")
                    icon.name: "list-add"
                    onClicked: {
                        const p = { name: pname.text || pname.placeholderText, kind: kind.currentIndex === 0 ? "anthropic" : "openai",
                                    model: pmodel.text || pmodel.placeholderText, baseUrl: kind.currentIndex === 1 ? (purl.text || purl.placeholderText) : "",
                                    fallback: kind.currentIndex === 0 ? "default" : "" }
                        page.patch({ providers: page.settings.providers.concat([p]) })
                        pname.text = ""; pmodel.text = ""; purl.text = ""
                    }
                }
            }
        }

        // ---------- Style ----------
        SectionLabel { text: i18n("Style"); Layout.topMargin: Kirigami.Units.gridUnit }
        QQC2.Label {
            Layout.fillWidth: true
            wrapMode: Text.Wrap
            color: KanteStyle.mutedTextColor
            text: i18n("System follows your Plasma colour scheme and is the default. Kante Light adds Kante's shapes and titles; Kante uses its own palette.")
        }
        Flow {
            Layout.fillWidth: true
            spacing: Kirigami.Units.largeSpacing
            QQC2.ButtonGroup { id: styles }
            Repeater {
                model: [["system", i18n("System")], ["kante-light", i18n("Kante Light")], ["kante", i18n("Kante")]]
                delegate: QQC2.AbstractButton {
                    id: styleChoice
                    required property var modelData
                    checkable: true
                    checked: page.settings.style === modelData[0]
                    QQC2.ButtonGroup.group: styles
                    onToggled: if (checked) page.patch({ style: modelData[0] })
                    implicitWidth: Kirigami.Units.gridUnit * 10
                    implicitHeight: preview.height + label.implicitHeight + Kirigami.Units.smallSpacing
                    hoverEnabled: true

                    StylePreview {
                        id: preview
                        kind: styleChoice.modelData[0]
                        selected: styleChoice.checked
                        hovered: styleChoice.hovered
                        width: parent.width
                    }
                    QQC2.Label {
                        id: label
                        anchors { top: preview.bottom; topMargin: Kirigami.Units.smallSpacing; horizontalCenter: parent.horizontalCenter }
                        text: styleChoice.modelData[1]
                        font.bold: styleChoice.checked
                    }
                }
            }
        }

        QQC2.Label {
            Layout.topMargin: Kirigami.Units.gridUnit
            text: i18n("Config: %1", page.settings.path || "")
            font: KanteStyle.monoFont(Kirigami.Theme.smallFont.pointSize)
            color: KanteStyle.mutedTextColor
        }
    }

    /** One check box column of the folder table. */
    component Cell: Item {
        id: cell
        property alias checked: box.checked
        property string tip
        signal toggled()
        implicitWidth: page.columnWidth
        implicitHeight: box.implicitHeight
        QQC2.CheckBox {
            id: box
            anchors.centerIn: parent
            onToggled: cell.toggled()
            QQC2.ToolTip.visible: hovered && cell.tip !== ""
            QQC2.ToolTip.text: cell.tip
            KanteCheckSkin { control: parent }
        }
    }

    /** A small picture of each style: its ground, a card and the accent. */
    component StylePreview: Item {
        id: sp
        property string kind
        property bool selected
        property bool hovered
        readonly property bool kante: kind !== "system"
        readonly property var pal: KanteStyle.light ? KantePalette.light : KantePalette.dark
        readonly property color ground: kind === "kante" ? pal.ground : Kirigami.Theme.backgroundColor
        readonly property color card: kind === "kante" ? pal.card : Kirigami.Theme.alternateBackgroundColor
        readonly property color text: kind === "kante" ? pal.text : Kirigami.Theme.textColor
        readonly property color accent: kind === "kante" ? pal.accent : Kirigami.Theme.highlightColor
        height: Kirigami.Units.gridUnit * 5.5

        Rectangle {
            anchors.fill: parent
            color: sp.ground
            radius: sp.kante ? 0 : Kirigami.Units.cornerRadius
            border.width: sp.selected ? 2 : 1
            border.color: sp.selected ? KanteStyle.accentColor : (sp.hovered ? KanteStyle.frameColor : Qt.alpha(sp.text, 0.2))
        }
        // A card with Kante's cut corner in the Kante styles.
        Canvas {
            id: cardShape
            x: Kirigami.Units.largeSpacing
            y: Kirigami.Units.largeSpacing
            width: parent.width - Kirigami.Units.largeSpacing * 2
            height: parent.height - Kirigami.Units.largeSpacing * 2
            readonly property real cut: sp.kante ? Kirigami.Units.gridUnit * 0.6 : 0
            onPaint: {
                const ctx = getContext("2d")
                ctx.reset()
                ctx.fillStyle = sp.card
                ctx.beginPath()
                ctx.moveTo(0, 0)
                ctx.lineTo(width - cut, 0)
                ctx.lineTo(width, cut)
                ctx.lineTo(width, height)
                ctx.lineTo(0, height)
                ctx.closePath()
                ctx.fill()
                ctx.fillStyle = sp.accent
                ctx.fillRect(0, 0, sp.kante ? width - cut : 3, sp.kante ? 2 : height)
            }
            Connections {
                target: sp
                function onCardChanged() { cardShape.requestPaint() }
                function onAccentChanged() { cardShape.requestPaint() }
            }
        }
        Column {
            x: Kirigami.Units.largeSpacing * 2
            y: Kirigami.Units.largeSpacing * 2
            spacing: Kirigami.Units.smallSpacing
            Text {
                text: i18n("Heading")
                color: sp.text
                font: sp.kante ? KanteStyle.kanteHeading(Kirigami.Theme.defaultFont.pointSize) : Qt.font({ family: Kirigami.Theme.defaultFont.family, pointSize: Kirigami.Theme.defaultFont.pointSize, bold: true })
            }
            Rectangle { width: Kirigami.Units.gridUnit * 5; height: 3; color: Qt.alpha(sp.text, 0.4) }
            Rectangle { width: Kirigami.Units.gridUnit * 3.5; height: 3; color: Qt.alpha(sp.text, 0.4) }
            Rectangle {
                width: Kirigami.Units.gridUnit * 3
                height: Kirigami.Units.gridUnit * 0.8
                color: sp.accent
                radius: sp.kante ? 0 : height / 4
            }
        }
    }
}
