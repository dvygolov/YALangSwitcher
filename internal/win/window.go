//go:build windows

package win

import (
	"errors"
	"syscall"
	"unsafe"
)

// WndProcFunc is the window procedure of the single window the app creates.
type WndProcFunc func(hwnd HWND, msg uint32, wparam, lparam uintptr) uintptr

var (
	wndHandler  WndProcFunc
	wndProcProc = syscall.NewCallback(wndProcDispatch)
)

func wndProcDispatch(hwnd HWND, msg uint32, wp, lp uintptr) (ret uintptr) {
	defer func() {
		// A panic must never cross back into the Win32 message loop: it would
		// abort the whole process instead of one message.
		if recover() != nil {
			ret = DefWindowProc(hwnd, msg, wp, lp)
		}
	}()
	if wndHandler != nil {
		return wndHandler(hwnd, msg, wp, lp)
	}
	return DefWindowProc(hwnd, msg, wp, lp)
}

// GetModuleHandle returns the HINSTANCE of the running exe.
func GetModuleHandle() HINSTANCE {
	r, _, _ := pGetModuleHandleW.Call(0)
	return HINSTANCE(r)
}

// RegisterClass registers the window class used by the hidden app window.
func RegisterClass(className string, icon HICON) error {
	inst := GetModuleHandle()
	cursor, _, _ := pLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   wndProcProc,
		HInstance:     inst,
		HIcon:         icon,
		HCursor:       cursor,
		LpszClassName: UTF16Ptr(className),
	}
	r, _, e := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		if errno, ok := e.(syscall.Errno); ok && errno == 1410 {
			return nil // class already registered, fine
		}
		if errno, ok := e.(syscall.Errno); ok && errno != 0 {
			return errno
		}
		return errors.New("RegisterClassExW failed")
	}
	return nil
}

// CreateHiddenWindow creates the (never shown) top level window that receives
// the tray callbacks, timers and hook events.
func CreateHiddenWindow(className, title string, handler WndProcFunc) HWND {
	wndHandler = handler
	r, _, _ := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(UTF16Ptr(className))),
		uintptr(unsafe.Pointer(UTF16Ptr(title))),
		0, // WS_OVERLAPPED: no caption, never shown
		0, 0, 0, 0,
		0, 0, uintptr(GetModuleHandle()), 0,
	)
	return HWND(r)
}

func DestroyWindow(h HWND) {
	if h != 0 {
		pDestroyWindow.Call(uintptr(h))
	}
}

func IsWindow(h HWND) bool {
	r, _, _ := pIsWindow.Call(uintptr(h))
	return r != 0
}

// MessageLoop pumps messages until WM_QUIT. It has to run on the same OS thread
// that created the window and installed the keyboard hook.
func MessageLoop() {
	var m MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func PostQuitMessage(code int32) { pPostQuitMessage.Call(uintptr(code)) }

func DefWindowProc(h HWND, msg uint32, wp, lp uintptr) uintptr {
	r, _, _ := pDefWindowProcW.Call(uintptr(h), uintptr(msg), wp, lp)
	return r
}

func PostMessage(h HWND, msg uint32, wp, lp uintptr) bool {
	r, _, _ := pPostMessageW.Call(uintptr(h), uintptr(msg), wp, lp)
	return r != 0
}

func FindWindow(className string) HWND {
	r, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(UTF16Ptr(className))), 0)
	return HWND(r)
}

// RegisterWindowMessage registers a broadcast message such as TaskbarCreated.
func RegisterWindowMessage(name string) uint32 {
	r, _, _ := pRegisterWindowMsg.Call(uintptr(unsafe.Pointer(UTF16Ptr(name))))
	return uint32(r)
}

func SetTimer(h HWND, id uintptr, ms uint32) {
	pSetTimer.Call(uintptr(h), id, uintptr(ms), 0)
}

func KillTimer(h HWND, id uintptr) { pKillTimer.Call(uintptr(h), id) }

func MessageBox(text, title string, flags uint32) int {
	r, _, _ := pMessageBoxW.Call(0,
		uintptr(unsafe.Pointer(UTF16Ptr(text))),
		uintptr(unsafe.Pointer(UTF16Ptr(title))),
		uintptr(flags))
	return int(r)
}

func GetSystemMetrics(index int) int32 {
	r, _, _ := pGetSystemMetrics.Call(uintptr(index))
	return int32(r)
}

func CursorPos() POINT {
	var p POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p
}

func SetForegroundWindow(h HWND) { pSetForegroundW.Call(uintptr(h)) }

func Sleep(ms uint32) { pSleep.Call(uintptr(ms)) }

// CreateInstanceMutex creates (or opens) the named single instance mutex. The
// second return value reports that another instance already created it.
func CreateInstanceMutex(name string) (HANDLE, bool) {
	r, _, e := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(UTF16Ptr(name))))
	errno, _ := e.(syscall.Errno)
	if r == 0 {
		return 0, true
	}
	return HANDLE(r), errno == ERROR_ALREADY_EXISTS
}

func CloseHandle(h HANDLE) {
	if h != 0 {
		pCloseHandle.Call(uintptr(h))
	}
}
