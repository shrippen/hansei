import QtQuick
import QtCore
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
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
                                    journal: Qt.resolvedUrl("JournalPage.qml"), settings: Qt.resolvedUrl("SettingsPage.qml") })
    property string current: "review"

    // System (platform theme) is the default; Kante and Kante Light are opt-in in the settings.
    Binding {
        target: KanteStyle
        property: "kind"
        value: store.style === "kante" ? KanteStyle.Kind.Kante : (store.style === "kante-light" ? KanteStyle.Kind.KanteLight : KanteStyle.Kind.System)
    }
    KanteScope { target: root.contentItem }
    color: KanteStyle.themed ? KanteStyle.backgroundColor : Kirigami.Theme.backgroundColor

    Store { id: store }

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

    function openTask(text, folders) {
        taskDialog.openWith(text || "", folders || null)
    }

    // Window layout that survives restarts: drawer and review column widths.
    Settings {
        id: layout
        category: "Layout"
        property real drawerWidth: -1
        property real batchListWidth: Kirigami.Units.gridUnit * 15
        property real feedbackWidth: Kirigami.Units.gridUnit * 19
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
                checked: root.current === "settings"
                onTriggered: root.show("settings")
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
    Shortcut { sequence: "Ctrl+,"; onActivated: root.show("settings") }

    // Connection problems and first start.
    Kirigami.InlineMessage {
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
