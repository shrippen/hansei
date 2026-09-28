#include "client.h"

#include <KLocalizedString>
#include <KNotification>

#include <QCoreApplication>
#include <QDir>
#include <QFileInfo>
#include <QGuiApplication>
#include <QJSEngine>
#include <QJsonDocument>
#include <QJsonObject>
#include <QProcess>
#include <QStandardPaths>

namespace
{
constexpr int retryMs = 250;
constexpr int maxAttempts = 60; // 15 s for the daemon to come up
constexpr auto daemonIdle = "15m";
}

Client::Client(QJSEngine *engine, QObject *parent)
    : QObject(parent)
    , m_engine(engine)
{
    // Callbacks hold engine values; drop them before the engine goes away on quit.
    connect(QCoreApplication::instance(), &QCoreApplication::aboutToQuit, this, [this] {
        m_pending.clear();
        m_socket.disconnect(this);
        m_socket.abort();
    });
    m_retry.setInterval(retryMs);
    m_retry.setSingleShot(true);
    connect(&m_retry, &QTimer::timeout, this, &Client::tryConnect);
    connect(&m_socket, &QLocalSocket::readyRead, this, &Client::onReadyRead);
    connect(&m_socket, &QLocalSocket::disconnected, this, &Client::onDisconnected);
    connect(&m_socket, &QLocalSocket::connected, this, [this] {
        m_attempts = 0;
        setProblem(QString());
        Q_EMIT connectedChanged();
        send(R"({"jsonrpc":"2.0","id":0,"method":"subscribe"})");
        const auto queued = m_queue;
        m_queue.clear();
        for (const auto &line : queued) {
            send(line);
        }
    });
    connect(&m_socket, &QLocalSocket::errorOccurred, this, [this](QLocalSocket::LocalSocketError) {
        if (m_socket.state() == QLocalSocket::ConnectedState) {
            return;
        }
        // No daemon: start one (once), then keep trying for a while.
        if (!m_launched) {
            launchDaemon();
        }
        if (++m_attempts < maxAttempts && !m_needsSetup) {
            m_retry.start();
            return;
        }
        if (!m_needsSetup) {
            setProblem(i18n("The Hansei service does not answer. Start “hansei daemon” in a terminal to see why."));
        }
    });
}

bool Client::connected() const
{
    return m_socket.state() == QLocalSocket::ConnectedState;
}

bool Client::demoBuild() const
{
#ifdef HANSEI_DEMO
    return true;
#else
    return false;
#endif
}

QString Client::version() const
{
    return QStringLiteral(HANSEI_VERSION);
}

QString Client::socketPath() const
{
    const QString env = qEnvironmentVariable("HANSEI_SOCKET");
    if (!env.isEmpty()) {
        return env;
    }
    QString dir = qEnvironmentVariable("XDG_RUNTIME_DIR");
    if (dir.isEmpty()) {
        dir = QDir::tempPath();
    }
    return dir + QStringLiteral("/hansei.sock");
}

// binary finds the Go program: $HANSEI_BIN, next to this app, or on PATH.
QString Client::binary() const
{
    const QString env = qEnvironmentVariable("HANSEI_BIN");
    if (!env.isEmpty()) {
        return env;
    }
    const QString beside = QCoreApplication::applicationDirPath() + QStringLiteral("/hansei");
    if (QFileInfo(beside).isExecutable()) {
        return beside;
    }
    return QStandardPaths::findExecutable(QStringLiteral("hansei"));
}

void Client::start()
{
    m_attempts = 0;
    m_launched = false;
    tryConnect();
}

void Client::tryConnect()
{
    if (m_socket.state() != QLocalSocket::UnconnectedState) {
        return;
    }
    m_socket.connectToServer(socketPath());
}

void Client::launchDaemon()
{
    m_launched = true;
    QString configPath = qEnvironmentVariable("HANSEI_CONFIG");
    if (configPath.isEmpty()) {
        configPath = QStandardPaths::writableLocation(QStandardPaths::GenericConfigLocation) + QStringLiteral("/hansei/config.yaml");
    }
    if (!QFileInfo::exists(configPath)) {
        setProblem(QString(), true); // first start: the setup page explains itself
        return;
    }
    const QString bin = binary();
    if (bin.isEmpty()) {
        setProblem(i18n("The program “hansei” was not found. Install it or set HANSEI_BIN."));
        return;
    }
    // Detached: AI tasks keep running when the window closes; the daemon quits when idle.
    if (!QProcess::startDetached(bin, {QStringLiteral("daemon"), QStringLiteral("--idle"), QString::fromLatin1(daemonIdle)})) {
        setProblem(i18n("Could not start “%1”.", bin));
    }
}

void Client::setup(const QString &vault, const QStringList &allow, const QStringList &block)
{
    const QString bin = binary();
    QProcess p;
    p.start(bin, {QStringLiteral("init"), QStringLiteral("--vault"), vault, QStringLiteral("--allow"), allow.join(QLatin1Char(',')),
                  QStringLiteral("--block"), block.join(QLatin1Char(','))});
    p.waitForFinished();
    if (p.exitCode() != 0) {
        setProblem(QString::fromUtf8(p.readAllStandardError()).trimmed(), true);
        return;
    }
    m_needsSetup = false;
    start();
}

void Client::setProblem(const QString &problem, bool setup)
{
    if (m_problem == problem && m_needsSetup == setup) {
        if (setup) {
            Q_EMIT problemChanged(); // show the setup page even if nothing else changed
        }
        return;
    }
    m_problem = problem;
    m_needsSetup = setup;
    Q_EMIT problemChanged();
}

void Client::call(const QString &method, const QVariantMap &params, const QJSValue &callback)
{
    const int id = ++m_next;
    if (callback.isCallable()) {
        m_pending.insert(id, callback);
    }
    QJsonObject req{{QStringLiteral("jsonrpc"), QStringLiteral("2.0")}, {QStringLiteral("id"), id}, {QStringLiteral("method"), method},
                    {QStringLiteral("params"), QJsonObject::fromVariantMap(params)}};
    const QByteArray line = QJsonDocument(req).toJson(QJsonDocument::Compact);
    if (!connected()) {
        m_queue.append(line);
        start();
        return;
    }
    send(line);
}

void Client::send(const QByteArray &line)
{
    m_socket.write(line + '\n');
    m_socket.flush();
}

void Client::onReadyRead()
{
    m_buffer += m_socket.readAll();
    int nl;
    while ((nl = m_buffer.indexOf('\n')) >= 0) {
        const QByteArray line = m_buffer.left(nl);
        m_buffer.remove(0, nl + 1);
        const QJsonObject msg = QJsonDocument::fromJson(line).object();

        if (msg.value(QStringLiteral("method")).toString() == QStringLiteral("event")) {
            const QJsonObject e = msg.value(QStringLiteral("params")).toObject();
            Q_EMIT event(e.value(QStringLiteral("kind")).toString(), e.value(QStringLiteral("batch")).toString(), e.value(QStringLiteral("text")).toString(),
                         e.value(QStringLiteral("path")).toString());
            continue;
        }
        const int id = msg.value(QStringLiteral("id")).toInt();
        auto it = m_pending.find(id);
        if (it == m_pending.end()) {
            continue;
        }
        QJSValue cb = it.value();
        m_pending.erase(it);
        const QJSValue result = m_engine->toScriptValue(msg.value(QStringLiteral("result")).toVariant());
        const QJsonValue err = msg.value(QStringLiteral("error"));
        const QJSValue error = err.isObject() ? QJSValue(err.toObject().value(QStringLiteral("message")).toString()) : QJSValue();
        cb.call({result, error});
    }
}

void Client::onDisconnected()
{
    // Fail waiting calls so the UI does not hang, then reconnect (the daemon may restart).
    const auto pending = m_pending;
    m_pending.clear();
    for (auto cb : pending) {
        cb.call({QJSValue(), QJSValue(i18n("Connection to the Hansei service lost."))});
    }
    Q_EMIT connectedChanged();
    m_launched = false;
    m_attempts = 0;
    m_retry.start();
}

void Client::notify(const QString &title, const QString &text)
{
    if (QGuiApplication::focusWindow()) {
        return;
    }
    auto *n = new KNotification(QStringLiteral("batchReady"));
    n->setTitle(title);
    n->setText(text);
    n->setIconName(QStringLiteral("org.shrippen.hansei"));
    n->sendEvent();
}

QString Client::startFolder() const
{
    const QStringList args = QCoreApplication::arguments();
    for (int i = 1; i < args.size(); ++i) {
        if (!args.at(i).startsWith(QLatin1Char('-'))) {
            return args.at(i);
        }
    }
    return QString();
}
