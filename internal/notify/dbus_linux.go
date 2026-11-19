//go:build linux

package notify

import (
	"context"
	"sync"

	"github.com/godbus/dbus/v5"
)

// dbusNotifier talks to org.freedesktop.Notifications and keeps replacing
// its own notification, so skipping tracks does not stack popups.
type dbusNotifier struct {
	mu   sync.Mutex
	conn *dbus.Conn
	id   uint32
}

func newDBusNotifier() Notifier {
	return &dbusNotifier{}
}

func (n *dbusNotifier) Notify(ctx context.Context, title, body, iconPath string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.conn == nil {
		conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
		if err != nil {
			return err
		}
		n.conn = conn
	}

	obj := n.conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	hints := map[string]dbus.Variant{
		"category":       dbus.MakeVariant("x-gnome.music"),
		"desktop-entry":  dbus.MakeVariant("spotui"),
		"suppress-sound": dbus.MakeVariant(true),
	}
	call := obj.CallWithContext(ctx, "org.freedesktop.Notifications.Notify", 0,
		"spotui", n.id, iconPath, title, body, []string{}, hints, int32(5000))
	if call.Err != nil {
		// The bus may have gone away; reconnect next time.
		_ = n.conn.Close()
		n.conn = nil
		return call.Err
	}
	_ = call.Store(&n.id)
	return nil
}
