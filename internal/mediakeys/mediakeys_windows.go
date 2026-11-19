//go:build windows

package mediakeys

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procPostThreadMsg    = user32.NewProc("PostThreadMessageW")
)

const (
	wmHotkey = 0x0312
	wmQuit   = 0x0012

	modNoRepeat = 0x4000
)

var vkKeys = map[uintptr]Key{
	0xB3: PlayPause, // VK_MEDIA_PLAY_PAUSE
	0xB0: Next,      // VK_MEDIA_NEXT_TRACK
	0xB1: Prev,      // VK_MEDIA_PREV_TRACK
	0xB2: Stop,      // VK_MEDIA_STOP
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
	_       [8]byte // lPrivate and slack; GetMessageW must not write past us
}

// Listen registers global hotkeys for the media keys and calls handler for
// each press until stop is called. Keys already taken by another program
// are skipped; it fails only if none could be registered.
func Listen(handler func(Key)) (stop func(), err error) {
	ready := make(chan error, 1)
	threadID := make(chan uint32, 1)

	go func() {
		// Hotkeys belong to the thread that registered them, and that same
		// thread must pump the messages.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		ids := map[uintptr]Key{}
		var id uintptr = 1
		for vk, key := range vkKeys {
			if r, _, _ := procRegisterHotKey.Call(0, id, modNoRepeat, vk); r != 0 {
				ids[id] = key
			}
			id++
		}
		if len(ids) == 0 {
			ready <- fmt.Errorf("media keys are taken by another program")
			return
		}
		threadID <- windows.GetCurrentThreadId()
		ready <- nil

		var m msg
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 { // WM_QUIT or error
				break
			}
			if m.message == wmHotkey {
				if key, ok := ids[m.wParam]; ok {
					handler(key)
				}
			}
		}
		for hid := range ids {
			procUnregisterHotKey.Call(0, hid)
		}
	}()

	if err := <-ready; err != nil {
		return func() {}, err
	}
	tid := <-threadID
	return func() { procPostThreadMsg.Call(uintptr(tid), wmQuit, 0, 0) }, nil
}
