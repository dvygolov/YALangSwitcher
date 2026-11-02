//go:build windows

package win

import (
	"syscall"
	"unsafe"
)

// Low level keyboard hook (WH_KEYBOARD_LL).
//
// It is called before the input reaches the foreground application, so it sees
// every keystroke of the session - including the ones going to elevated
// windows, as long as this process itself is elevated. Nothing is swallowed:
// the hook only observes.

// KeyEvent is a normalised keyboard event.
type KeyEvent struct {
	VK       uint32
	Scan     uint32
	Down     bool
	Injected bool
	// Ours marks input synthesised by this very process (the fallback hotkey).
	Ours bool
}

// KeyHandler observes every key. It must return fast: it runs inside the input
// path of the whole system.
type KeyHandler func(KeyEvent)

var (
	keyHandler KeyHandler
	keyHookCB  = syscall.NewCallback(keyboardProc)
	hookHandle HANDLE
)

func keyboardProc(nCode int32, wparam, lparam uintptr) uintptr {
	if nCode == HC_ACTION && keyHandler != nil {
		kb := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lparam))
		down := wparam == WM_KEYDOWN_ || wparam == WM_SYSKEYDOWN_
		keyHandler(KeyEvent{
			VK:       kb.VkCode,
			Scan:     kb.ScanCode,
			Down:     down,
			Injected: kb.Flags&LLKHF_INJECTED != 0,
			Ours:     kb.DwExtraInfo == MagicInjectedInput,
		})
	}
	r, _, _ := pCallNextHookEx.Call(uintptr(hookHandle), uintptr(nCode), wparam, lparam)
	return r
}

const (
	WM_KEYDOWN_    = 0x0100
	WM_KEYUP_      = 0x0101
	WM_SYSKEYDOWN_ = 0x0104
	WM_SYSKEYUP_   = 0x0105
)

// InstallKeyboardHook installs the hook. It must be called from the thread that
// runs the message loop; the hook is torn down when that thread exits.
func InstallKeyboardHook(handler KeyHandler) error {
	keyHandler = handler
	h, _, e := pSetWindowsHookExW.Call(WH_KEYBOARD_LL, keyHookCB, uintptr(GetModuleHandle()), 0)
	if h == 0 {
		if errno, ok := e.(syscall.Errno); ok && errno != 0 {
			return errno
		}
		return syscall.EINVAL
	}
	hookHandle = HANDLE(h)
	return nil
}

func RemoveKeyboardHook() {
	if hookHandle != 0 {
		pUnhookWindowsHookEx.Call(uintptr(hookHandle))
		hookHandle = 0
	}
	keyHandler = nil
}

// KeyDown reports whether the key is currently held, as seen by the input
// queue (modifiers included).
func KeyDown(vk uintptr) bool {
	r, _, _ := pGetAsyncKeyState.Call(vk)
	return uint16(r)&0x8000 != 0
}

// SendHotkey synthesises a language switch hotkey such as alt+shift. It is the
// fallback for windows that ignore WM_INPUTLANGCHANGEREQUEST.
func SendHotkey(name string) bool {
	var keys []uintptr
	switch name {
	case "alt+shift":
		keys = []uintptr{VK_MENU, VK_SHIFT}
	case "ctrl+shift":
		keys = []uintptr{VK_CONTROL, VK_SHIFT}
	case "win+space":
		keys = []uintptr{VK_LWIN, VK_SPACE}
	default:
		return false
	}

	// every modifier is pressed with a fresh down/up pair so that stuck keys
	// from a previous attempt cannot confuse the shell hotkey handler
	var seq []INPUT
	for _, k := range keys {
		seq = append(seq, keyInput(k, false))
	}
	for i := len(keys) - 1; i >= 0; i-- {
		seq = append(seq, keyInput(keys[i], true))
	}
	if len(seq) == 0 {
		return false
	}
	r, _, _ := pSendInput.Call(uintptr(len(seq)), uintptr(unsafe.Pointer(&seq[0])), unsafe.Sizeof(seq[0]))
	return int(r) == len(seq)
}

func keyInput(vk uintptr, up bool) INPUT {
	flags := uint32(0)
	if up {
		flags = KEYEVENTF_KEYUP
	}
	return INPUT{Type: 1 /* INPUT_KEYBOARD */, Ki: KEYBDINPUT{
		WvK:         uint16(vk),
		DwFlags:     flags,
		DwExtraInfo: MagicInjectedInput,
	}}
}
