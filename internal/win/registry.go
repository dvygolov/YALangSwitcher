//go:build windows

package win

import (
	"strings"
	"syscall"
	"unsafe"
)

// Small registry helpers: autostart (HKCU\...\Run) and the Windows 11
// NotifyIconSettings tweak that keeps the tray icon visible on the taskbar.

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

func regCreate(path string) HKEY {
	var h HKEY
	var disp uint32
	r, _, _ := pRegCreateKeyExW.Call(
		HKEY_CURRENT_USER,
		uintptr(unsafe.Pointer(UTF16Ptr(path))),
		0, 0, 0, KEY_WRITE,
		0, uintptr(unsafe.Pointer(&h)), uintptr(unsafe.Pointer(&disp)))
	if r != 0 {
		return 0
	}
	return h
}

func regOpen(path string, access uintptr) HKEY {
	var h HKEY
	r, _, _ := pRegOpenKeyExW.Call(
		HKEY_CURRENT_USER,
		uintptr(unsafe.Pointer(UTF16Ptr(path))),
		0, access, uintptr(unsafe.Pointer(&h)))
	if r != 0 {
		return 0
	}
	return h
}

func regClose(h HKEY) {
	if h != 0 {
		pRegCloseKey.Call(uintptr(h))
	}
}

func regSetString(h HKEY, name, value string) bool {
	v := syscall.StringToUTF16(value)
	r, _, _ := pRegSetValueExW.Call(uintptr(h),
		uintptr(unsafe.Pointer(UTF16Ptr(name))), 0, REG_SZ,
		uintptr(unsafe.Pointer(&v[0])), uintptr(len(v)*2))
	return r == 0
}

func regSetDword(h HKEY, name string, value uint32) bool {
	r, _, _ := pRegSetValueExW.Call(uintptr(h),
		uintptr(unsafe.Pointer(UTF16Ptr(name))), 0, REG_DWORD,
		uintptr(unsafe.Pointer(&value)), 4)
	return r == 0
}

// SetRunValue writes an autostart entry for the current user. args is appended
// to the quoted exe path, e.g. "--start-task".
func SetRunValue(name, exePath, args string) bool {
	h := regCreate(runKeyPath)
	if h == 0 {
		return false
	}
	defer regClose(h)
	value := `"` + exePath + `"`
	if args != "" {
		value += " " + args
	}
	return regSetString(h, name, value)
}

func DeleteRunValue(name string) {
	h := regOpen(runKeyPath, KEY_WRITE)
	if h == 0 {
		return
	}
	defer regClose(h)
	pRegDeleteValueW.Call(uintptr(h), uintptr(unsafe.Pointer(UTF16Ptr(name))))
}

func RunValue(name string) string {
	var buf [1024]uint16
	size := uint32(len(buf) * 2)
	r, _, _ := pRegGetValueW.Call(HKEY_CURRENT_USER,
		uintptr(unsafe.Pointer(UTF16Ptr(runKeyPath))),
		uintptr(unsafe.Pointer(UTF16Ptr(name))),
		RRF_RT_REG_SZ, 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:])
}

// PromoteTrayIcon marks the icon as "always show" in Windows 11's tray, so the
// app does not disappear into the overflow flyout after the first start.
func PromoteTrayIcon(exePath string) bool {
	if exePath == "" {
		return false
	}
	const root = `Control Panel\NotifyIconSettings`
	hkey := regOpen(root, KEY_READ)
	if hkey == 0 {
		return false
	}
	defer regClose(hkey)

	buf := make([]uint16, 256)
	done := false
	for i := uint32(0); ; i++ {
		n := uint32(len(buf))
		rr, _, _ := pRegEnumKeyExW.Call(uintptr(hkey), uintptr(i),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)), 0, 0, 0, 0)
		if rr != 0 {
			break
		}
		sub := root + `\` + syscall.UTF16ToString(buf[:n])

		var pathBuf [1024]uint16
		size := uint32(len(pathBuf) * 2)
		pr, _, _ := pRegGetValueW.Call(HKEY_CURRENT_USER,
			uintptr(unsafe.Pointer(UTF16Ptr(sub))),
			uintptr(unsafe.Pointer(UTF16Ptr("ExecutablePath"))),
			RRF_RT_REG_SZ, 0,
			uintptr(unsafe.Pointer(&pathBuf[0])), uintptr(unsafe.Pointer(&size)))
		if pr != 0 {
			continue
		}
		if !strings.EqualFold(syscall.UTF16ToString(pathBuf[:]), exePath) {
			continue
		}
		kh := regOpen(sub, KEY_WRITE)
		if kh == 0 {
			continue
		}
		if regSetDword(kh, "IsPromoted", 1) {
			done = true
		}
		regClose(kh)
	}
	return done
}
