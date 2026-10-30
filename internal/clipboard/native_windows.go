//go:build windows

package clipboard

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	emptyClipboard   = user32.NewProc("EmptyClipboard")
	setClipboardData = user32.NewProc("SetClipboardData")
	globalAlloc      = kernel32.NewProc("GlobalAlloc")
	globalFree       = kernel32.NewProc("GlobalFree")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
	moveMemory       = kernel32.NewProc("RtlMoveMemory")
)

// nativeCopy puts text on the Windows clipboard as CF_UNICODETEXT.
func nativeCopy(text string) error {
	// The clipboard is owned per thread between Open and Close.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Another process may hold the clipboard briefly; retry for ~500ms.
	var opened bool
	for i := 0; i < 10; i++ {
		if r, _, _ := openClipboard.Call(0); r != 0 {
			opened = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !opened {
		return fmt.Errorf("OpenClipboard başarısız")
	}
	defer closeClipboard.Call() //nolint:errcheck // nothing to do on failure

	if r, _, err := emptyClipboard.Call(); r == 0 {
		return fmt.Errorf("EmptyClipboard: %w", err)
	}

	utf16, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	size := uintptr(len(utf16) * 2)
	h, _, err := globalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return fmt.Errorf("GlobalAlloc: %w", err)
	}
	p, _, err := globalLock.Call(h)
	if p == 0 {
		globalFree.Call(h) //nolint:errcheck
		return fmt.Errorf("GlobalLock: %w", err)
	}
	moveMemory.Call(p, uintptr(unsafe.Pointer(&utf16[0])), size) //nolint:errcheck // RtlMoveMemory returns nothing
	globalUnlock.Call(h)                                         //nolint:errcheck

	if r, _, err := setClipboardData.Call(cfUnicodeText, h); r == 0 {
		globalFree.Call(h) //nolint:errcheck
		return fmt.Errorf("SetClipboardData: %w", err)
	}
	// The system owns h after a successful SetClipboardData.
	return nil
}
