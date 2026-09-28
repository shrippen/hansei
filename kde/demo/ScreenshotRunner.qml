import QtQuick

// Demo builds only: walks the shots of demo/shots.json ($SHOT_PLAN) and saves each window
// grab as $SHOT_DIR/<name>.png, then quits. Does nothing without a plan.
Item {
    id: runner

    readonly property var shots: (typeof ShotPlan !== "undefined" && ShotPlan) ? (ShotPlan.shots || ShotPlan) : []
    property int index: -1
    property var keys: []
    property var target: null

    function step() {
        index++
        if (index >= shots.length) {
            Qt.quit()
            return
        }
        const s = shots[index]
        if (s.manual) {
            step()
            return
        }
        const win = applicationWindow()
        if (s.width) {
            win.width = s.width
        }
        if (s.height) {
            win.height = s.height
        }
        if (s.batch) {
            win.store.batchID = s.batch
            win.store.path = s.path || ""
        }
        if (s.mode) {
            win.store.mode = s.mode
        }
        if (s.page) {
            win.show(s.page)
        }
        if (s.task) {
            win.openTask()
        }
        // "settings": module id; the shot is taken of the settings window.
        target = win
        if (s.settings) {
            win.openSettings(s.settings)
        }
        keys = s.keys || []
        grab.interval = s.wait || 1500
        if (keys.length > 0) {
            keyTimer.start()
            return
        }
        grab.start()
    }

    // Keys of a shot, one every 700 ms, then the grab.
    Timer {
        id: keyTimer
        interval: 700
        repeat: true
        onTriggered: {
            if (runner.keys.length === 0) {
                stop()
                grab.start()
                return
            }
            const k = runner.keys[0]
            // "type:…" types text, "scroll:N" scrolls the page to N px, anything else is a key sequence.
            if (k.startsWith("scroll:")) {
                const cfg = applicationWindow().settingsView.configViewItem
                const stack = cfg && cfg.visible && cfg.pageStack ? cfg.pageStack : applicationWindow().pageStack
                const page = stack.depth > 0 ? stack.get(stack.depth - 1) : null
                if (page && page.flickable) {
                    page.flickable.contentY = Number(k.slice(7))
                }
            } else if (k.startsWith("type:")) {
                Shots.type(applicationWindow(), k.slice(5))
            } else {
                Shots.press(applicationWindow(), k)
            }
            runner.keys = runner.keys.slice(1)
        }
    }

    Timer {
        id: grab
        onTriggered: {
            const s = runner.shots[runner.index]
            const w = s.settings && applicationWindow().settingsView.configViewItem ? applicationWindow().settingsView.configViewItem : applicationWindow()
            Shots.grab(w, ShotDir + "/" + s.name + ".png")
            if (s.settings && w !== applicationWindow()) {
                w.close()
            }
            runner.step()
        }
    }

    Timer {
        running: runner.shots.length > 0 && (Hansei.connected || Hansei.needsSetup)
        interval: 2500
        onTriggered: runner.step()
    }
}
