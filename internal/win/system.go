//go:build windows

package win

import (
	"syscall"
	"unsafe"
)

// IsElevated reports whether the process runs with an elevated token. The
// keyboard hook only sees input that is sent to elevated windows when we are
// elevated ourselves, so this decides how the app starts.
func IsElevated() bool {
	var tok HANDLE
	r, _, _ := pOpenProcessToken.Call(uintptr(CurrentProcess()), TOKEN_QUERY,
		uintptr(unsafe.Pointer(&tok)))
	if r == 0 {
		return false
	}
	defer pCloseHandle.Call(uintptr(tok))
	var elevated, retLen uint32
	r, _, _ = pGetTokenInformation.Call(uintptr(tok), TokenElevation,
		uintptr(unsafe.Pointer(&elevated)), 4, uintptr(unsafe.Pointer(&retLen)))
	return r != 0 && elevated != 0
}

// CurrentProcess returns the pseudo handle of the current process.
func CurrentProcess() HANDLE {
	r, _, _ := pGetCurrentProcess.Call()
	return HANDLE(r)
}

// ExePath returns the full path of the running executable.
func ExePath() string {
	buf := make([]uint16, 32768)
	h := uintptr(CurrentProcess())
	n := uint32(len(buf))
	r, _, _ := pQueryFullProcessImageW.Call(h, 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 || n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// ShellExecute opens a file, folder or URL through the shell.
func ShellExecute(verb, file, params, dir string, show int32) bool {
	var verbPtr, paramsPtr, dirPtr uintptr
	if verb != "" {
		verbPtr = uintptr(unsafe.Pointer(UTF16Ptr(verb)))
	}
	if params != "" {
		paramsPtr = uintptr(unsafe.Pointer(UTF16Ptr(params)))
	}
	if dir != "" {
		dirPtr = uintptr(unsafe.Pointer(UTF16Ptr(dir)))
	}
	r, _, _ := pShellExecuteW.Call(0,
		verbPtr,
		uintptr(unsafe.Pointer(UTF16Ptr(file))),
		paramsPtr, dirPtr, uintptr(show))
	return r > 32
}

// ForegroundWindow returns the window that currently has the input focus.
func ForegroundWindow() HWND {
	r, _, _ := pGetForegroundWindow.Call()
	return HWND(r)
}

// WindowThread returns the thread id that owns the window (and thus its
// keyboard layout).
func WindowThread(h HWND) uint32 {
	var pid uint32
	r, _, _ := pGetWindowThreadPID.Call(uintptr(h), uintptr(unsafe.Pointer(&pid)))
	return uint32(r)
}

// ThreadLayout returns the active keyboard layout (HKL) of a thread.
func ThreadLayout(threadID uint32) uintptr {
	r, _, _ := pGetKeyboardLayout.Call(uintptr(threadID))
	return r
}

// WindowLayout returns the active keyboard layout of the window's thread.
func WindowLayout(h HWND) uintptr {
	tid := WindowThread(h)
	if tid == 0 {
		return 0
	}
	return ThreadLayout(tid)
}

// Layouts lists the layouts installed for the current user, in cycling order.
func Layouts() []uintptr {
	r, _, _ := pGetKeyboardLayouts.Call(0, 0)
	n := int32(r)
	if n <= 0 || n > 64 {
		return nil
	}
	list := make([]uintptr, n)
	pGetKeyboardLayouts.Call(uintptr(n), uintptr(unsafe.Pointer(&list[0])))
	return list
}

// LoadLayout loads a layout by its KLID ("00000409") and returns its handle.
func LoadLayout(klid string) uintptr {
	r, _, _ := pLoadKeyboardLayoutW.Call(
		uintptr(unsafe.Pointer(UTF16Ptr(klid))), KLF_ACTIVATE)
	return r
}

// LangID extracts the Windows language id from a layout handle, e.g. 0x0409 for
// en-US and 0x0419 for ru-RU (0 when hkl is invalid).
func LangID(hkl uintptr) uint32 {
	return uint32(hkl) & 0xFFFF
}

// PrimaryLang is the primary language part of a language id: both en-US (0x09)
// and en-GB (0x09) map to LANG_ENGLISH, ru-RU to LANG_RUSSIAN.
func PrimaryLang(hkl uintptr) uint32 {
	return LangID(hkl) & 0x3FF
}

// PostLayoutChange asks a window to switch to another keyboard layout. The
// message is handled by DefWindowProc in the receiving thread, which activates
// the layout for that thread - i.e. for the application the user is typing in.
func PostLayoutChange(h HWND, hkl uintptr) bool {
	if h == 0 || hkl == 0 {
		return false
	}
	return PostMessage(h, WM_INPUTLANGCHANGEREQUEST, 0, hkl)
}
