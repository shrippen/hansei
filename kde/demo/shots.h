#pragma once

#include <QObject>

class QQmlApplicationEngine;
class QQuickWindow;

// Demo builds only: hands $SHOT_PLAN and $SHOT_DIR to demo/ScreenshotRunner.qml and grabs the window.
class Shots : public QObject
{
    Q_OBJECT
public:
    using QObject::QObject;
    static void install(QQmlApplicationEngine &engine);
    Q_INVOKABLE bool grab(QQuickWindow *window, const QString &path);
    /** Presses a key sequence like "A" or "Shift+J" in the window (to walk through a flow). */
    Q_INVOKABLE void press(QQuickWindow *window, const QString &sequence);
    /** Types text into the focused item, character by character. */
    Q_INVOKABLE void type(QQuickWindow *window, const QString &text);
};
