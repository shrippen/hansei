import QtQuick
import QtCore
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import org.kde.kirigamiaddons.settings as KirigamiSettings
import Kante
import "components"

Kirigami.ApplicationWindow {
    id: root

    title: i18n("Hansei")
    width: Kirigami.Units.gridUnit * 78
    height: Kirigami.Units.gridUnit * 46
    minimumWidth: Kirigami.Units.gridUnit * 24
    minimumHeight: Kirigami.Units.gridUnit * 20

    property alias store: store
    // Pages look the app state up through the window.
    readonly property var pages: ({ start: Qt.resolvedUrl("StartPage.qml"), review: Qt.resolvedUrl("ReviewPage.qml"), board: Qt.resolvedUrl("BoardPage.qml"),
                                    journal: Qt.resolvedUrl("JournalPage.qml") })
    property string current: "review"

    // System (platform theme) is the default; Kante and Kante Light are opt-in in the settings.
    Binding {
        target: KanteStyle
        property: "kind"
        value: store.style === "kante" ? KanteStyle.Kind.Kante : (store.style === "kante-light" ? KanteStyle.Kind.KanteLight : KanteStyle.Kind.System)
    }
    KanteScope { target: root.contentItem }
    color: KanteStyle.themed ? KanteStyle.backgroundColor : Kirigami.Theme.backgroundColor

    Store { id: store; window: root }

    function show(name) {
        current = name
        taskDialog.close()
        // Layers (e.g. the history of a change) sit above the pages: close them first.
        while (pageStack.layers.depth > 1) {
            pageStack.layers.pop()
        }
        if (pageStack.depth === 0) {
            pageStack.push(pages[name])
            return
        }
        while (pageStack.depth > 1) {
            pageStack.pop()
        }
        pageStack.replace(pages[name])
    }

    // Settings: Kirigami's settings window, one page per category.
    KirigamiSettings.ConfigurationView {
        id: settingsView
        window: root
        modules: [
            KirigamiSettings.ConfigurationModule {
                moduleId: "folders"
                text: i18n("Folders")
                icon.name: "folder"
                page: () => Qt.createComponent(Qt.resolvedUrl("SettingsPage.qml"))
                initialProperties: () => ({ store: store, section: "folders" })
            },
            KirigamiSettings.ConfigurationModule {
                moduleId: "checks"
                text: i18n("Checks")
                icon.name: "checkmark"
                page: () => Qt.createComponent(Qt.resolvedUrl("SettingsPage.qml"))
                initialProperties: () => ({ store: store, section: "checks" })
            },
            KirigamiSettings.ConfigurationModule {
                moduleId: "providers"
                text: i18n("AI providers")
                icon.name: "network-server"
                page: () => Qt.createComponent(Qt.resolvedUrl("SettingsPage.qml"))
                initialProperties: () => ({ store: store, section: "providers" })
            },
            KirigamiSettings.ConfigurationModule {
                moduleId: "look"
                text: i18n("Appearance")
                icon.name: "preferences-desktop-theme-global"
                page: () => Qt.createComponent(Qt.resolvedUrl("SettingsPage.qml"))
                initialProperties: () => ({ store: store, section: "look" })
            }
        ]
    }
    Component { id: kanteScope; KanteScope {} }

    function openSettings(module) {
        settingsView.open(module || "")
        Qt.callLater(() => {
            // The settings window follows the chosen style like the main window.
            const win = settingsView.configViewItem
            if (win && win.contentItem && root.styledSettings !== win) {
                kanteScope.createObject(win.contentItem, { target: win.contentItem })
                win.color = Qt.binding(() => KanteStyle.themed ? KanteStyle.backgroundColor : Kirigami.Theme.backgroundColor)
                root.styledSettings = win
            }
        })
    }
    property alias settingsView: settingsView
    property var styledSettings: null

    function openTask(text, folders) {
        taskDialog.openWith(text || "", folders || null)
    }

    // Window layout that survives restarts: drawer and review column widths.
    Settings {
        id: layout
        category: "Layout"
        // Demo runs (screenshots) never touch your real layout.
        location: Hansei.demoBuild ? StandardPaths.writableLocation(StandardPaths.TempLocation) + "/hansei-demo-layout.conf" : ""
        property real drawerWidth: -1
        property real batchListWidth: Kirigami.Units.gridUnit * 15
        property real feedbackWidth: Kirigami.Units.gridUnit * 19
        property bool feedbackFolded: false
        property string recentTasks: "[]"
        property int sessions: 0
    }
    property alias layout: layout

    // "Review with Hansei…" from Dolphin: a folder inside the vault opens a task with that scope.
    property bool startFolderDone: false
    function openStartFolder() {
        const folder = Hansei.startFolder()
        if (startFolderDone || !folder) {
            return
        }
        startFolderDone = true
        store.call("status", {}, st => {
            if (!st || !folder.startsWith(st.vault + "/")) {
                return
            }
            let rel = folder.slice(st.vault.length + 1)
            if (rel.endsWith(".md")) {
                rel = rel.split("/").slice(0, -1).join("/")
            }
            taskDialog.openWith("", rel ? [rel] : [])
        })
    }

    function toast(text, actionText, action) {
        if (!text) {
            return
        }
        // A page with its own action bar at the bottom shows messages at the top instead.
        const page = pageStack.currentItem
        if (page && typeof page.showNotice === "function" && page.showNotice(text, action)) {
            return
        }
        if (actionText) {
            showPassiveNotification(text, 8000, actionText, action)
        } else {
            showPassiveNotification(text)
        }
    }

    globalDrawer: Kirigami.GlobalDrawer {
        id: drawer
        // Narrow windows: icons only below ~1000 px, an overlay on phone widths.
        modal: root.width < Kirigami.Units.gridUnit * 30
        collapsible: true
        collapsed: root.width < 1000
        // The width can be dragged at the edge and is remembered.
        interactiveResizeEnabled: !collapsed && !modal
        preferredSize: layout.drawerWidth
        onPreferredSizeChanged: if (preferredSize > 0) layout.drawerWidth = preferredSize
        showHeaderWhenCollapsed: true
        header: Brand { collapsed: drawer.collapsed }

        actions: [
            Kirigami.Action {
                text: i18n("Start")
                icon.name: "go-home"
                checked: root.current === "start"
                onTriggered: root.show("start")
            },
            Kirigami.Action {
                text: store.waiting > 0 ? i18n("Review (%1)", store.waiting) : i18n("Review")
                icon.name: "document-compare"
                checked: root.current === "review"
                onTriggered: root.show("review")
            },
            Kirigami.Action {
                text: i18n("Workbench")
                icon.name: "view-list-details"
                checked: root.current === "board"
                onTriggered: root.show("board")
            },
            Kirigami.Action {
                text: i18n("Journal")
                icon.name: "view-history"
                checked: root.current === "journal"
                onTriggered: root.show("journal")
            },
            Kirigami.Action {
                text: i18n("Settings")
                icon.name: "configure"
                onTriggered: root.openSettings()
            },
            Kirigami.Action {
                separator: true
            },
            Kirigami.Action {
                text: i18n("New task…")
                icon.name: "list-add"
                shortcut: "N"
                onTriggered: root.openTask()
            }
        ]
    }

    readonly property url setupPage: Qt.resolvedUrl("SetupPage.qml")

    TaskDialog { id: taskDialog }

    Shortcut { sequence: "W"; enabled: !store.typing; onActivated: root.show(root.current === "board" ? "review" : "board") }
    Shortcut { sequence: "S"; enabled: !store.typing; onActivated: root.show("start") }
    Shortcut { sequence: "O"; enabled: !store.typing; onActivated: root.show("journal") }
    Shortcut { sequence: "Ctrl+,"; onActivated: root.openSettings() }

    // Connection problems and first start.
    Kirigami.InlineMessage {
        KanteMessageSkin { message: parent }
        id: problem
        z: 10
        anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: Kirigami.Units.largeSpacing }
        type: Kirigami.MessageType.Warning
        visible: Hansei.problem !== "" && !Hansei.needsSetup
        text: Hansei.problem
        actions: Kirigami.Action {
            text: i18n("Try again")
            icon.name: "view-refresh"
            onTriggered: Hansei.start()
        }
    }

    Connections {
        target: Hansei
        function onProblemChanged() {
            if (Hansei.needsSetup && root.current !== "setup") {
                root.current = "setup"
                pageStack.clear()
                pageStack.push(setupPage)
            }
        }
        function onConnectedChanged() {
            if (!Hansei.connected) {
                return
            }
            store.refresh()
            if (pageStack.depth === 0 || root.current === "setup") {
                root.show(root.current === "setup" ? "start" : root.current)
            }
            root.openStartFolder()
        }
    }

    Component.onCompleted: {
        layout.sessions = layout.sessions + 1
        if (Hansei.needsSetup) {
            current = "setup"
            pageStack.push(setupPage)
            return
        }
        show("review")
    }

    // Demo builds only: takes the screenshots listed in $SHOT_PLAN.
    Loader {
        active: Hansei.demoBuild
        source: Hansei.demoBuild ? "qrc:/demo/ScreenshotRunner.qml" : ""
    }
}
