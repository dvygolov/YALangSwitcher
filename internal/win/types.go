//go:build windows

package win

import "syscall"

type HWND uintptr
type HICON uintptr
type HMENU uintptr
type HINSTANCE uintptr
type HANDLE uintptr
type HKEY uintptr

type POINT struct{ X, Y int32 }

type MSG struct {
	HWnd    HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	Private uint32
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     HINSTANCE
	HIcon         HICON
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       HICON
}

type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            HICON
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     HICON
}

type KBDLLHOOKSTRUCT struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// INPUT / KEYBDINPUT for SendInput. The struct has to match the native layout
// exactly: 40 bytes on amd64 (type + padding + union).
type KEYBDINPUT struct {
	WvK         uint16
	WvScan      uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type INPUT struct {
	Type uint32
	_    uint32
	Ki   KEYBDINPUT
	_    [8]byte
}

func UTF16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return nil
	}
	return p
}

func CopyUTF16(dst []uint16, s string) {
	if len(dst) == 0 {
		return
	}
	n := len(dst) - 1
	i := 0
	for _, r := range s {
		if i >= n {
			break
		}
		if r > 0xFFFF {
			r = '?'
		}
		dst[i] = uint16(r)
		i++
	}
	dst[i] = 0
}
