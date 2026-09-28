#include "shots.h"

#include <QCoreApplication>
#include <QImage>
#include <QKeyEvent>
#include <QKeySequence>
#include <QJsonDocument>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickWindow>

void Shots::install(QQmlApplicationEngine &engine)
{
    const QByteArray plan = qgetenv("SHOT_PLAN");
    engine.rootContext()->setContextProperty(QStringLiteral("ShotPlan"), QJsonDocument::fromJson(plan).toVariant());
    engine.rootContext()->setContextProperty(QStringLiteral("ShotDir"), QString::fromLocal8Bit(qgetenv("SHOT_DIR")));
    engine.rootContext()->setContextProperty(QStringLiteral("Shots"), new Shots(&engine));
}

bool Shots::grab(QQuickWindow *window, const QString &path)
{
    return window && window->grabWindow().save(path);
}

void Shots::press(QQuickWindow *window, const QString &sequence)
{
    const QKeyCombination combo = QKeySequence(sequence)[0];
    // Only single letters carry text; named keys (Escape, Return) do not.
    const QString key = sequence.section(QLatin1Char('+'), -1);
    const QString text = key.size() != 1 ? QString() : (combo.keyboardModifiers() & Qt::ShiftModifier ? key.toUpper() : key.toLower());
    QCoreApplication::sendEvent(window, new QKeyEvent(QEvent::ShortcutOverride, combo.key(), combo.keyboardModifiers(), text));
    QKeyEvent press(QEvent::KeyPress, combo.key(), combo.keyboardModifiers(), text);
    QCoreApplication::sendEvent(window, &press);
    QKeyEvent release(QEvent::KeyRelease, combo.key(), combo.keyboardModifiers(), text);
    QCoreApplication::sendEvent(window, &release);
}

void Shots::type(QQuickWindow *window, const QString &text)
{
    for (const QChar c : text) {
        QKeyEvent press(QEvent::KeyPress, 0, Qt::NoModifier, QString(c));
        QCoreApplication::sendEvent(window, &press);
        QKeyEvent release(QEvent::KeyRelease, 0, Qt::NoModifier, QString(c));
        QCoreApplication::sendEvent(window, &release);
    }
}
