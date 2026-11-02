//go:build windows

// Package win is the minimal set of Win32 bindings YALangSwitcher needs: a
// hidden window with a message loop, a tray icon, a low level keyboard hook,
// keyboard layout switching and a handful of registry/system helpers.
//
// Everything is loaded lazily from system DLLs through syscall.NewLazyDLL:
// no CGO, no third party packages, a ~1 MB exe.
package win

import "syscall"

var (
	dllUser32   = syscall.NewLazyDLL("user32.dll")
	dllKernel32 = syscall.NewLazyDLL("kernel32.dll")
	dllShell32  = syscall.NewLazyDLL("shell32.dll")
	dllAdvapi32 = syscall.NewLazyDLL("advapi32.dll")
)

// kernel32
var (
	pGetModuleHandleW       = dllKernel32.NewProc("GetModuleHandleW")
	pGetCurrentProcess      = dllKernel32.NewProc("GetCurrentProcess")
	pCreateMutexW           = dllKernel32.NewProc("CreateMutexW")
	pOpenMutexW             = dllKernel32.NewProc("OpenMutexW")
	pCloseHandle            = dllKernel32.NewProc("CloseHandle")
	pQueryFullProcessImageW = dllKernel32.NewProc("QueryFullProcessImageNameW")
	pSleep                  = dllKernel32.NewProc("Sleep")
)

// user32
var (
	pRegisterClassExW  = dllUser32.NewProc("RegisterClassExW")
	pCreateWindowExW   = dllUser32.NewProc("CreateWindowExW")
	pDefWindowProcW    = dllUser32.NewProc("DefWindowProcW")
	pDestroyWindow     = dllUser32.NewProc("DestroyWindow")
	pIsWindow          = dllUser32.NewProc("IsWindow")
	pGetMessageW       = dllUser32.NewProc("GetMessageW")
	pTranslateMessage  = dllUser32.NewProc("TranslateMessage")
	pDispatchMessageW  = dllUser32.NewProc("DispatchMessageW")
	pPostQuitMessage   = dllUser32.NewProc("PostQuitMessage")
	pPostMessageW      = dllUser32.NewProc("PostMessageW")
	pFindWindowW       = dllUser32.NewProc("FindWindowW")
	pRegisterWindowMsg = dllUser32.NewProc("RegisterWindowMessageW")
	pSetTimer          = dllUser32.NewProc("SetTimer")
	pKillTimer         = dllUser32.NewProc("KillTimer")
	pMessageBoxW       = dllUser32.NewProc("MessageBoxW")
	pGetCursorPos      = dllUser32.NewProc("GetCursorPos")
	pSetForegroundW    = dllUser32.NewProc("SetForegroundWindow")
	pGetSystemMetrics  = dllUser32.NewProc("GetSystemMetrics")
	pLoadCursorW       = dllUser32.NewProc("LoadCursorW")
	pDestroyIcon       = dllUser32.NewProc("DestroyIcon")
	pCreateIconFromRes = dllUser32.NewProc("CreateIconFromResourceEx")
	pCreatePopupMenu   = dllUser32.NewProc("CreatePopupMenu")
	pAppendMenuW       = dllUser32.NewProc("AppendMenuW")
	pSetMenuDefault    = dllUser32.NewProc("SetMenuDefaultItem")
	pTrackPopupMenu    = dllUser32.NewProc("TrackPopupMenu")
	pDestroyMenu       = dllUser32.NewProc("DestroyMenu")

	// keyboard: global low level hook + injected input for the fallback
	pSetWindowsHookExW   = dllUser32.NewProc("SetWindowsHookExW")
	pCallNextHookEx      = dllUser32.NewProc("CallNextHookEx")
	pUnhookWindowsHookEx = dllUser32.NewProc("UnhookWindowsHookEx")
	pSendInput           = dllUser32.NewProc("SendInput")
	pGetAsyncKeyState    = dllUser32.NewProc("GetAsyncKeyState")

	// input language (keyboard layout) of a window
	pGetForegroundWindow = dllUser32.NewProc("GetForegroundWindow")
	pGetWindowThreadPID  = dllUser32.NewProc("GetWindowThreadProcessId")
	pGetKeyboardLayout   = dllUser32.NewProc("GetKeyboardLayout")
	pGetKeyboardLayouts  = dllUser32.NewProc("GetKeyboardLayoutList")
	pLoadKeyboardLayoutW = dllUser32.NewProc("LoadKeyboardLayoutW")
)

// shell32
var (
	pShellNotifyIconW = dllShell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW    = dllShell32.NewProc("ShellExecuteW")
)

// advapi32
var (
	pOpenProcessToken    = dllAdvapi32.NewProc("OpenProcessToken")
	pGetTokenInformation = dllAdvapi32.NewProc("GetTokenInformation")
	pRegCreateKeyExW     = dllAdvapi32.NewProc("RegCreateKeyExW")
	pRegSetValueExW      = dllAdvapi32.NewProc("RegSetValueExW")
	pRegGetValueW        = dllAdvapi32.NewProc("RegGetValueW")
	pRegDeleteValueW     = dllAdvapi32.NewProc("RegDeleteValueW")
	pRegOpenKeyExW       = dllAdvapi32.NewProc("RegOpenKeyExW")
	pRegEnumKeyExW       = dllAdvapi32.NewProc("RegEnumKeyExW")
	pRegCloseKey         = dllAdvapi32.NewProc("RegCloseKey")
)

// Constants used across the package.
const (
	WM_NULL                           = 0x0000
	WM_DESTROY                        = 0x0002
	WM_CLOSE                          = 0x0010
	WM_COMMAND                        = 0x0111
	WM_TIMER                          = 0x0113
	WM_INPUTLANGCHANGEREQUEST         = 0x0050
	WM_LBUTTONUP                      = 0x0202
	WM_RBUTTONUP                      = 0x0205
	WM_CONTEXTMENU                    = 0x007B
	WM_APP                            = 0x8000
	ERROR_ALREADY_EXISTS              = 183
	SM_CXSMICON                       = 49
	SM_CYSMICON                       = 50
	KLF_ACTIVATE                      = 0x0001
	WH_KEYBOARD_LL                    = 13
	HC_ACTION                         = 0
	KEYEVENTF_KEYUP                   = 0x0002
	TOKEN_QUERY                       = 0x0008
	TokenElevation                    = 20
	NIM_ADD                           = 0
	NIM_MODIFY                        = 1
	NIM_DELETE                        = 2
	NIF_MESSAGE                       = 0x00000001
	NIF_ICON                          = 0x00000002
	NIF_TIP                           = 0x00000004
	NIF_INFO                          = 0x00000010
	NIIF_INFO                         = 0x00000001
	NIIF_WARNING                      = 0x00000002
	MF_STRING                         = 0x00000000
	MF_SEPARATOR                      = 0x00000800
	MF_CHECKED                        = 0x00000008
	MF_GRAYED                         = 0x00000001
	MF_DISABLED                       = 0x00000002
	TPM_RETURNCMD                     = 0x0100
	TPM_RIGHTBUTTON                   = 0x0002
	TPM_NONOTIFY                      = 0x0080
	MB_OK                             = 0x00000000
	MB_ICONINFORMATION                = 0x00000040
	MB_ICONWARNING                    = 0x00000030
	SW_SHOWNORMAL                     = 1
	IDC_ARROW                         = 32512
	CREATE_ICON_VERSION               = 0x00030000
	HKEY_CURRENT_USER                 = uintptr(0x80000001)
	KEY_READ                          = 0x00020019
	KEY_WRITE                         = 0x00020006
	REG_SZ                            = 1
	REG_DWORD                         = 4
	RRF_RT_REG_SZ                     = 0x00000002
	RRF_RT_REG_DWORD                  = 0x00000010
	VK_LBUTTON                        = 0x01
	VK_RBUTTON                        = 0x02
	VK_SHIFT                          = 0x10
	VK_CONTROL                        = 0x11
	VK_MENU                           = 0x12
	VK_SPACE                          = 0x20
	VK_LWIN                           = 0x5B
	VK_RWIN                           = 0x5C
	VK_LSHIFT                         = 0xA0
	VK_RSHIFT                         = 0xA1
	LLKHF_INJECTED                    = 0x10
	KBD_SCAN_LSHIFT                   = 0x2A
	KBD_SCAN_RSHIFT                   = 0x36
	LANG_ENGLISH                      = 0x09
	LANG_RUSSIAN                      = 0x19
	MagicInjectedInput        uintptr = 0x59414C47 // 'YALG' - marks our own SendInput
)
