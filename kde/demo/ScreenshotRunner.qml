import QtQuick

// Demo builds only: walks the shots of demo/shots.json ($SHOT_PLAN) and saves each window
// grab as $SHOT_DIR/<name>.png, then quits. Does nothing without a plan.
Item {
    id: runner

    readonly property var shots: (typeof ShotPlan !== "undefined" && ShotPlan) ? (ShotPlan.shots || ShotPlan) : []
    property int index: -1
    property var keys: []

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
            // "type:…" types text, anything else is a key sequence.
            if (k.startsWith("type:")) {
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
            Shots.grab(applicationWindow(), ShotDir + "/" + s.name + ".png")
            runner.step()
        }
    }

    Timer {
        running: runner.shots.length > 0 && (Hansei.connected || Hansei.needsSetup)
        interval: 2500
        onTriggered: runner.step()
    }
}
