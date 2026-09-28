import QtQuick
import Kante
/**
 * Shared app state, refreshed from the daemon and its events.
 * Pages reach it as applicationWindow().store.
 */
QtObject {
    id: store

    property var status: ({})
    property var home: ({ stats: {}, findings: [], titles: {} })
    property var batches: []
    property var findings: ({ findings: [], summary: [] })   // full rule report, loaded on demand
    property var progress: ({})   // batch id → last tool step of the AI
    property var aiText: ({})     // batch id → streamed answer text
    property var liveUsage: ({})  // batch id → token use while the AI works
    property bool typing: false   // a text field has focus: single-letter shortcuts are off

    // Review selection survives page switches.
    property string batchID: ""
    property string path: ""
    property string mode: "split"
    property bool reveal: false

    readonly property string style: status.style || "system"
    readonly property string lang: status.lang || "en"
    readonly property var providers: status.providers || []
    readonly property var defaultProvider: {
        for (const p of providers) {
            if (p.default) {
                return p
            }
        }
        return null
    }
    readonly property int waiting: {
        let n = 0
        for (const b of batches) {
            if (b.column === "review" || b.column === "feedback") {
                n++
            }
        }
        return n
    }

    signal batchChanged(string id)
    signal settingsChanged()

    property var _running: ({})

    function call(method, params, cb) {
        Hansei.call(method, params || {}, function (result, error) {
            if (error) {
                applicationWindow().toast(error)
            }
            if (cb) {
                cb(result, error)
            }
        })
    }

    function refresh() {
        call("status", {}, r => { if (r) status = r })
        refreshHome()
        refreshBatches()
    }

    function refreshHome() {
        call("home", {}, r => { if (r) home = r })
    }

    function refreshFindings(force) {
        call("findings", { refresh: !!force }, r => { if (r) findings = r })
    }

    function findingsOf(rule) {
        return (findings.findings || []).filter(f => f.rule === rule)
    }

    // Severity of a check: secrets are urgent, stale review dates only informative.
    function ruleColor(rule) {
        switch (rule) {
        case "secret": return KanteStyle.negativeTextColor
        case "codename":
        case "frontmatter": return KanteStyle.neutralTextColor
        case "review": return KanteStyle.infoColor
        default: return KanteStyle.tagColor
        }
    }

    // obsidian://open for a vault path.
    function obsidianUrl(path) {
        return "obsidian://open?vault=" + encodeURIComponent(status.vaultName || "") + "&file=" + encodeURIComponent(path.replace(/\.md$/, ""))
    }

    function fileName(path) {
        return (path || "").split("/").pop()
    }

    // Short relative age: "2 min", "3 h", "4 d".
    function age(time) {
        const s = Math.max(0, (Date.now() - new Date(time).getTime()) / 1000)
        if (s < 3600) {
            return i18np("one minute ago", "%1 minutes ago", Math.max(1, Math.round(s / 60)))
        }
        if (s < 86400) {
            return i18np("one hour ago", "%1 hours ago", Math.round(s / 3600))
        }
        return i18np("one day ago", "%1 days ago", Math.round(s / 86400))
    }

    function money(usage, currency) {
        if (!usage || !usage.cost) {
            return ""
        }
        return usage.cost.toLocaleString(Qt.locale(), "f", 2) + " " + (currency === "EUR" ? "€" : "$")
    }

    // Token use of a batch, live while the AI works.
    function usageOf(b) {
        if (!b) {
            return null
        }
        return (b.running || b.revising) && liveUsage[b.id] ? liveUsage[b.id] : b.usage
    }

    function currencyOf(b) {
        const name = b && b.provider ? b.provider : (defaultProvider ? defaultProvider.name : "")
        const p = providers.find(p => p.name === name)
        return p ? p.currency : ""
    }

    function tokens(usage) {
        const n = usage ? (usage.in || 0) + (usage.out || 0) : 0
        return n >= 1000 ? i18n("%1 k tokens", Math.round(n / 1000)) : i18n("%1 tokens", n)
    }

    function refreshBatches() {
        call("batches", {}, r => {
            if (!r) {
                return
            }
            // Tell the desktop when an AI run finished while the window was in the background.
            const was = _running
            const now = {}
            for (const b of r) {
                now[b.id] = b.running
                if (was[b.id] && !b.running) {
                    Hansei.notify(b.title, b.column === "failed" ? b.error : i18np("One file ready for review", "%1 files ready for review", b.counts.files))
                }
            }
            _running = now
            batches = r
        })
    }

    function batch(id) {
        for (const b of batches) {
            if (b.id === id) {
                return b
            }
        }
        return null
    }

    property Connections events: Connections {
        target: Hansei
        function onEvent(kind, id, text, path) {
            switch (kind) {
            case "progress": {
                const p = Object.assign({}, store.progress)
                p[id] = text
                store.progress = p
                break
            }
            case "ai": {
                const a = Object.assign({}, store.aiText)
                a[id] = ((a[id] || "") + text).slice(-400)
                store.aiText = a
                break
            }
            case "usage": {
                const u = Object.assign({}, store.liveUsage)
                u[id] = JSON.parse(text)
                store.liveUsage = u
                break
            }
            case "toast":
                if (path) {
                    // A file was written: undo right from the toast.
                    applicationWindow().toast(text, i18n("Undo"), () => store.call("undoFile", { id: id, path: path }))
                } else {
                    applicationWindow().toast(text)
                }
                break
            case "settings":
                store.refresh()
                store.settingsChanged()
                break
            case "vault":
                store.refreshHome()
                if ((store.findings.findings || []).length > 0) {
                    store.refreshFindings()
                }
                break
            case "batch": {
                const a = Object.assign({}, store.aiText)
                delete a[id]
                store.aiText = a
                store.refreshBatches()
                store.refreshHome()
                store.batchChanged(id)
                break
            }
            }
        }
    }
}
