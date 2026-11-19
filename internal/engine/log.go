package engine

import (
	librespot "github.com/devgianlu/go-librespot"
	"github.com/sirupsen/logrus"
)

// logAdapter exposes a logrus entry as a librespot.Logger.
type logAdapter struct {
	entry *logrus.Entry
}

// NewLogger wraps a logrus logger for go-librespot.
func NewLogger(l *logrus.Logger) librespot.Logger {
	return logAdapter{entry: logrus.NewEntry(l)}
}

func (l logAdapter) Tracef(format string, args ...any) { l.entry.Tracef(format, args...) }
func (l logAdapter) Debugf(format string, args ...any) { l.entry.Debugf(format, args...) }
func (l logAdapter) Infof(format string, args ...any)  { l.entry.Infof(format, args...) }
func (l logAdapter) Warnf(format string, args ...any)  { l.entry.Warnf(format, args...) }
func (l logAdapter) Errorf(format string, args ...any) { l.entry.Errorf(format, args...) }
func (l logAdapter) Trace(args ...any)                 { l.entry.Trace(args...) }
func (l logAdapter) Debug(args ...any)                 { l.entry.Debug(args...) }
func (l logAdapter) Info(args ...any)                  { l.entry.Info(args...) }
func (l logAdapter) Warn(args ...any)                  { l.entry.Warn(args...) }
func (l logAdapter) Error(args ...any)                 { l.entry.Error(args...) }

func (l logAdapter) WithField(key string, value any) librespot.Logger {
	return logAdapter{entry: l.entry.WithField(key, value)}
}

func (l logAdapter) WithError(err error) librespot.Logger {
	return logAdapter{entry: l.entry.WithError(err)}
}
