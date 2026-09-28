#pragma once

#include <QHash>
#include <QJSValue>
#include <QLocalSocket>
#include <QObject>
#include <QPointer>
#include <QTimer>

class QJSEngine;

/**
 * Connection to the Hansei daemon (JSON-RPC 2.0, one JSON object per line, Unix socket).
 * Starts the daemon when none answers, and can write a first config (hansei init).
 *
 *   QML ── call("decide", {...}, cb) ──▶ socket ──▶ hansei daemon
 *   QML ◀── event(kind, batch, text, path) ◀── "event" notifications
 */
class Client : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool connected READ connected NOTIFY connectedChanged)
    Q_PROPERTY(QString problem READ problem NOTIFY problemChanged)
    Q_PROPERTY(bool needsSetup READ needsSetup NOTIFY problemChanged)
    Q_PROPERTY(bool demoBuild READ demoBuild CONSTANT)
    Q_PROPERTY(QString version READ version CONSTANT)

public:
    explicit Client(QJSEngine *engine, QObject *parent = nullptr);

    bool connected() const;
    QString problem() const { return m_problem; }
    bool needsSetup() const { return m_needsSetup; }
    bool demoBuild() const;
    QString version() const;

    /** Calls a method; callback(result, error) runs on the answer. */
    Q_INVOKABLE void call(const QString &method, const QVariantMap &params, const QJSValue &callback = QJSValue());
    /** Connects, starting the daemon if needed. */
    Q_INVOKABLE void start();
    /** Writes a first config: hansei init --vault PATH --allow … --block …, then starts. */
    Q_INVOKABLE void setup(const QString &vault, const QStringList &allow, const QStringList &block);
    /** Desktop notification (only when the window is not active). */
    Q_INVOKABLE void notify(const QString &title, const QString &text);
    /** A folder argument from the command line or Dolphin ("Mit Hansei prüfen"). */
    Q_INVOKABLE QString startFolder() const;
    // A shortcut as the desktop writes it in the current language ("Shift+A" → "Umschalt+A").
    Q_INVOKABLE QString keyName(const QString &sequence) const;

Q_SIGNALS:
    void connectedChanged();
    void problemChanged();
    void event(const QString &kind, const QString &batch, const QString &text, const QString &path);

private:
    void onReadyRead();
    void onDisconnected();
    void tryConnect();
    void launchDaemon();
    void setProblem(const QString &problem, bool setup = false);
    void send(const QByteArray &line);
    QString binary() const;
    QString socketPath() const;

    QJSEngine *m_engine;
    QLocalSocket m_socket;
    QTimer m_retry;
    QByteArray m_buffer;
    QHash<int, QJSValue> m_pending;
    QList<QByteArray> m_queue;
    int m_next = 0;
    int m_attempts = 0;
    bool m_launched = false;
    bool m_needsSetup = false;
    QString m_problem;
};
