package logger

import (
	"fmt"
	"io"
	"os"
)

// Log functions wrapping slog.Logger

func Debug(msg string, args ...any) {
	GetLogger().Debug(msg, args...)
}

func Info(msg string, args ...any) {
	GetLogger().Info(msg, args...)
}

func Warn(msg string, args ...any) {
	GetLogger().Warn(msg, args...)
}

func Error(msg string, args ...any) {
	GetLogger().Error(msg, args...)
}

func Fatal(msg string, args ...any) {
	GetLogger().Error(msg, args...)
	os.Exit(1)
}

func Print(args ...any) {
	GetLogger().Info(fmt.Sprint(args...))
}

func Printf(format string, args ...any) {
	GetLogger().Info(fmt.Sprintf(format, args...))
}

func Println(args ...any) {
	GetLogger().Info(fmt.Sprint(args...))
}

func Fatalf(format string, args ...any) {
	GetLogger().Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

func Debugf(format string, args ...any) {
	GetLogger().Debug(fmt.Sprintf(format, args...))
}

func Infof(format string, args ...any) {
	GetLogger().Info(fmt.Sprintf(format, args...))
}

func Warnf(format string, args ...any) {
	GetLogger().Warn(fmt.Sprintf(format, args...))
}

func Errorf(format string, args ...any) {
	GetLogger().Error(fmt.Sprintf(format, args...))
}

func SetOutput(w io.Writer) {
	globalSafeWriter.Set(w)
}

func Writer() io.Writer {
	return globalSafeWriter.Get()
}
