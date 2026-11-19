//go:build !linux

package notify

func newDBusNotifier() Notifier { return nil }
