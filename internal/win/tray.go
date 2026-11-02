//go:build windows

package win

import "unsafe"

// Tray icon (Shell_NotifyIcon) and the context menu shown from it.

type Tray struct {
	data NOTIFYICONDATAW
	hwnd HWND
	icon HICON
}

// NewTray registers a tray icon whose mouse events are posted to cbMsg.
func NewTray(hwnd HWND, icon HICON, tip string, cbMsg uint32) *Tray {
	t := &Tray{hwnd: hwnd, icon: icon}
	d := &t.data
	d.CbSize = uint32(unsafe.Sizeof(NOTIFYICONDATAW{}))
	d.HWnd = hwnd
	d.UID = 1
	d.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	d.UCallbackMessage = cbMsg
	d.HIcon = icon
	CopyUTF16(d.SzTip[:], tip)
	t.Add()
	return t
}

// Add (re)adds the icon; Explorer drops it on restart, so this is also the
// handler for the TaskbarCreated broadcast.
func (t *Tray) Add() bool {
	r, _, _ := pShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&t.data)))
	return r != 0
}

// SetIcon swaps the current icon (the badge follows the active language).
func (t *Tray) SetIcon(icon HICON) {
	if icon == 0 || icon == t.icon {
		return
	}
	t.icon = icon
	t.data.HIcon = icon
	t.data.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	pShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&t.data)))
}

// SetTip updates the hover tooltip.
func (t *Tray) SetTip(tip string) {
	CopyUTF16(t.data.SzTip[:], tip)
	t.data.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	pShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&t.data)))
}

// Balloon shows a notification bubble.
func (t *Tray) Balloon(title, text string, flags uint32) {
	d := &t.data
	d.UFlags = NIF_INFO
	CopyUTF16(d.SzInfo[:], text)
	CopyUTF16(d.SzInfoTitle[:], title)
	d.DwInfoFlags = flags
	pShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(d)))
	d.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
}

func (t *Tray) Remove() {
	pShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&t.data)))
}

// Menu is a lightweight popup menu wrapper.
type Menu struct{ h HMENU }

func NewPopupMenu() *Menu {
	r, _, _ := pCreatePopupMenu.Call()
	return &Menu{h: HMENU(r)}
}

func (m *Menu) Add(id uint32, text string, checked bool) {
	flags := uint32(MF_STRING)
	if checked {
		flags |= MF_CHECKED
	}
	pAppendMenuW.Call(uintptr(m.h), uintptr(flags), uintptr(id), uintptr(unsafe.Pointer(UTF16Ptr(text))))
}

// AddInfo adds a disabled line used for the "current language" header.
func (m *Menu) AddInfo(text string) {
	pAppendMenuW.Call(uintptr(m.h), MF_STRING|MF_GRAYED|MF_DISABLED, 0, uintptr(unsafe.Pointer(UTF16Ptr(text))))
}

func (m *Menu) Separator() {
	pAppendMenuW.Call(uintptr(m.h), MF_SEPARATOR, 0, 0)
}

func (m *Menu) Default(id uint32) { pSetMenuDefault.Call(uintptr(m.h), uintptr(id), 0) }

func (m *Menu) Destroy() {
	if m.h != 0 {
		pDestroyMenu.Call(uintptr(m.h))
		m.h = 0
	}
}

// Track runs the menu at the cursor and returns the chosen command id (0 when
// dismissed).
func (m *Menu) Track(owner HWND) uint32 {
	pt := CursorPos()
	SetForegroundWindow(owner)
	r, _, _ := pTrackPopupMenu.Call(uintptr(m.h),
		TPM_RETURNCMD|TPM_RIGHTBUTTON|TPM_NONOTIFY,
		uintptr(pt.X), uintptr(pt.Y), 0, uintptr(owner), 0)
	PostMessage(owner, WM_NULL, 0, 0)
	return uint32(r)
}

// LoadIconFromICO picks the image closest to want pixels from an .ico file and
// returns a GDI icon handle.
func LoadIconFromICO(data []byte, want int) HICON {
	if len(data) < 6 || data[2] != 1 || data[3] != 0 {
		return 0
	}
	count := int(uint16(data[4]) | uint16(data[5])<<8)
	best, bestScore := 0, 1<<30
	for i := 0; i < count; i++ {
		off := 6 + 16*i
		if off+16 > len(data) {
			break
		}
		w := int(data[off])
		if w == 0 {
			w = 256
		}
		// classic bitmaps only: CreateIconFromResourceEx handles those on
		// every Windows version, PNG payloads are not needed for small sizes
		if w > 64 {
			continue
		}
		score := w - want
		if score < 0 {
			score = -score
		}
		if score < bestScore {
			bestScore, best = score, i
		}
	}
	if bestScore == 1<<30 {
		// no classic entry: fall back to the first one
		best = 0
	}
	off := 6 + 16*best
	if off+16 > len(data) {
		return 0
	}
	size := int(uint32(data[off+8]) | uint32(data[off+9])<<8 | uint32(data[off+10])<<16 | uint32(data[off+11])<<24)
	imgOff := int(uint32(data[off+12]) | uint32(data[off+13])<<8 | uint32(data[off+14])<<16 | uint32(data[off+15])<<24)
	if size <= 0 || imgOff <= 0 || imgOff+size > len(data) {
		return 0
	}
	ptr := unsafe.Pointer(&data[imgOff])
	h, _, _ := pCreateIconFromRes.Call(
		uintptr(ptr), uintptr(size), 1, CREATE_ICON_VERSION, 0, 0, 0)
	return HICON(h)
}

func DestroyIcon(h HICON) {
	if h != 0 {
		pDestroyIcon.Call(uintptr(h))
	}
}

// SmallIconSize is the size Windows wants for tray and window icons.
func SmallIconSize() int {
	n := GetSystemMetrics(SM_CXSMICON)
	if n <= 0 {
		return 16
	}
	return int(n)
}
