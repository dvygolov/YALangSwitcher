//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dvygolov/yalangswitcher/internal/win"
)

const (
	appName     = "YALangSwitcher"
	windowClass = "YALangSwitcherWnd"
	mutexName   = `Local\YALangSwitcherSingleInstance`
	taskName    = "YALangSwitcher"
	repoURL     = "https://github.com/dvygolov/YALangSwitcher"

	// window messages
	umTrayIcon     = win.WM_APP + 1
	umSwitch       = win.WM_APP + 2
	umSync         = win.WM_APP + 3
	umUpdateReady  = win.WM_APP + 11
	umUpdateFailed = win.WM_APP + 12

	// timers
	timerSync   = 1
	timerUpdate = 2

	// menu commands
	cmdAutostart = 101
	cmdAdmin     = 102
	cmdCheck     = 103
	cmdInstall   = 104
	cmdFolder    = 105
	cmdAbout     = 106
	cmdExit      = 107
)

// Badges are compiled into the exe so the tray icon never depends on a file
// next to the binary.
//
//go:embed assets/icon-en.ico
var iconENData []byte

//go:embed assets/icon-ru.ico
var iconRUData []byte

//go:embed assets/app.ico
var iconAppData []byte

// version is injected by the build: -ldflags "-X main.version=1.2.3".
var version = "0.0.0-dev"

var (
	iconEN  win.HICON
	iconRU  win.HICON
	iconApp win.HICON
)

// App is the whole program: one hidden window, one tray icon, one keyboard
// hook.
type App struct {
	hwnd win.HWND
	tray *win.Tray
	cfg  Config
	sw   *Switcher

	lang          Lang
	taskbarMsg    uint32
	promoteTries  int
	instanceMutex win.HANDLE
	firstRun      bool

	mu            sync.Mutex
	updateBusy    bool
	updateManual  bool
	updateVersion string
	updatePath    string
	updateMsg     string
}

func main() {
	// The message loop and the WH_KEYBOARD_LL hook have to live on one OS
	// thread, so pin the goroutine before anything else happens.
	runtime.LockOSThread()

	for _, arg := range os.Args[1:] {
		switch strings.ToLower(arg) {
		case "--version", "-v", "/version":
			win.MessageBox(appName+" "+version, appName, win.MB_OK|win.MB_ICONINFORMATION)
			return
		case "--help", "-h", "/?":
			win.MessageBox(helpText(), appName, win.MB_OK|win.MB_ICONINFORMATION)
			return
		}
	}

	cfg, firstRun := loadConfig()
	if hasArg(os.Args[1:], "--first-run") {
		firstRun = true
	}
	logf("start: %s %s (elevated=%v, pid=%d, exe=%s)",
		appName, version, win.IsElevated(), os.Getpid(), win.ExePath())
	logf("config: mode=%s fallback=%s en=%s ru=%s autostart=%v admin=%v",
		cfg.Mode, cfg.FallbackHotkey, cfg.EnglishLayout, cfg.RussianLayout,
		cfg.Autostart, cfg.AdminMode)

	if win.FindWindow(windowClass) != 0 {
		logf("start: another instance already owns the window")
		return
	}

	// Elevated apps do not receive input through a medium integrity keyboard
	// hook, so the switcher runs elevated itself. At logon the scheduled task
	// starts it silently; a manual start asks for UAC once.
	if cfg.AdminMode && !win.IsElevated() {
		exe := win.ExePath()
		params := ""
		if firstRun {
			params = "--first-run"
		}
		if exe != "" && win.ShellExecute("runas", exe, params, filepath.Dir(exe), win.SW_SHOWNORMAL) {
			logf("start: relaunching with administrator rights")
			return
		}
		logf("start: elevation was cancelled, running without administrator rights")
	}

	mutex, ok := claimInstance(mutexName, 8*time.Second)
	if !ok {
		logf("start: another instance holds the single instance mutex")
		return
	}

	app := &App{cfg: cfg, firstRun: firstRun, instanceMutex: mutex}
	app.run()
	closeLog()
}

func hasArg(args []string, name string) bool {
	for _, a := range args {
		if strings.EqualFold(a, name) {
			return true
		}
	}
	return false
}

// claimInstance takes the single instance slot. It waits briefly because a
// hand-over from the unelevated launcher to the elevated copy leaves the mutex
// alive for a moment.
func claimInstance(name string, timeout time.Duration) (win.HANDLE, bool) {
	deadline := time.Now().Add(timeout)
	for {
		h, exists := win.CreateInstanceMutex(name)
		if !exists {
			return h, true
		}
		if h != 0 {
			win.CloseHandle(h)
		}
		if time.Now().After(deadline) {
			return 0, false
		}
		win.Sleep(150)
	}
}

func (a *App) run() {
	iconEN = win.LoadIconFromICO(iconENData, win.SmallIconSize())
	iconRU = win.LoadIconFromICO(iconRUData, win.SmallIconSize())
	iconApp = win.LoadIconFromICO(iconAppData, 32)
	logf("icons: small=%d en=%d ru=%d app=%d",
		win.SmallIconSize(), iconEN, iconRU, iconApp)

	if err := win.RegisterClass(windowClass, iconApp); err != nil {
		logf("window: RegisterClass failed: %v", err)
	}
	a.hwnd = win.CreateHiddenWindow(windowClass, appName, a.wndProc)
	if a.hwnd == 0 {
		logf("window: CreateWindowEx failed")
		win.MessageBox("Не удалось создать окно приложения.\nПодробности: "+logPath(),
			appName, win.MB_OK|win.MB_ICONWARNING)
		return
	}
	a.taskbarMsg = win.RegisterWindowMessage("TaskbarCreated")

	a.sw = &Switcher{cfg: &a.cfg}
	a.sw.emit = func(l Lang) { win.PostMessage(a.hwnd, umSwitch, uintptr(l), 0) }

	a.tray = win.NewTray(a.hwnd, a.iconFor(langEN), a.tip(langEN), umTrayIcon)
	a.syncTray()

	if err := win.InstallKeyboardHook(a.onKey); err != nil {
		logf("hook: WH_KEYBOARD_LL failed: %v", err)
		a.balloon("Не удалось перехватить клавиатуру",
			"Система отклонила установку хука. Попробуйте запустить приложение от администратора.",
			win.NIIF_WARNING)
	} else {
		logf("hook: installed, mode=%s fallback=%s", a.cfg.Mode, a.cfg.FallbackHotkey)
	}

	a.syncAutostart()
	cleanupOldBinary()

	win.SetTimer(a.hwnd, timerSync, 1500)
	if a.cfg.AutoUpdate {
		win.SetTimer(a.hwnd, timerUpdate, 10_000)
	}

	if a.firstRun {
		a.balloon("YALangSwitcher запущен",
			"Левый Shift — English, правый Shift — Русский. Настройки и лог: "+dataDir(),
			win.NIIF_INFO)
	}
	if !win.IsElevated() && a.cfg.AdminMode {
		a.balloon("Нет прав администратора",
			"Клавиши не будут перехватываться в приложениях, запущенных от имени администратора.",
			win.NIIF_WARNING)
	}

	win.MessageLoop()
}

func (a *App) wndProc(hwnd win.HWND, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case a.taskbarMsg:
		// Explorer restarted: bring the icon back.
		if a.tray != nil {
			a.tray.Add()
			a.syncTray()
		}
		return 0

	case umTrayIcon:
		switch uint32(lp) {
		case win.WM_LBUTTONUP, win.WM_RBUTTONUP, win.WM_CONTEXTMENU:
			a.showMenu()
		}
		return 0

	case umSwitch:
		a.activate(Lang(wp))
		return 0

	case umSync:
		a.syncTray()
		return 0

	case win.WM_TIMER:
		switch wp {
		case timerSync:
			a.syncTray()
			a.promoteTrayIcon()
		case timerUpdate:
			win.KillTimer(hwnd, timerUpdate)
			go a.checkUpdates(false)
		}
		return 0

	case umUpdateFailed:
		a.mu.Lock()
		msg, manual := a.updateMsg, a.updateManual
		a.mu.Unlock()
		if manual && msg != "" {
			a.balloon("Обновления", msg, win.NIIF_INFO)
		}
		a.rearmUpdateTimer()
		return 0

	case umUpdateReady:
		a.mu.Lock()
		ver := a.updateVersion
		a.mu.Unlock()
		logf("update: %s downloaded, installing", ver)
		if err := a.applyUpdate(); err != nil {
			logf("update: apply failed: %v", err)
			a.balloon("Обновление "+ver, err.Error(), win.NIIF_WARNING)
			a.rearmUpdateTimer()
		}
		return 0

	case win.WM_CLOSE:
		win.DestroyWindow(hwnd)
		return 0

	case win.WM_DESTROY:
		win.KillTimer(hwnd, timerSync)
		win.KillTimer(hwnd, timerUpdate)
		win.RemoveKeyboardHook()
		if a.tray != nil {
			a.tray.Remove()
		}
		if a.instanceMutex != 0 {
			win.CloseHandle(a.instanceMutex)
			a.instanceMutex = 0
		}
		win.PostQuitMessage(0)
		return 0
	}
	return win.DefWindowProc(hwnd, msg, wp, lp)
}

// onKey runs inside the low level keyboard hook.
func (a *App) onKey(e win.KeyEvent) { a.sw.handle(e) }

func (a *App) iconFor(l Lang) win.HICON {
	if l == langRU {
		return iconRU
	}
	return iconEN
}

func (a *App) tip(l Lang) string {
	return fmt.Sprintf("%s — %s\nЛевый Shift → English, правый Shift → Русский", appName, l)
}

// syncTray repaints the tray badge so it always shows the language of the
// window that currently has the focus.
func (a *App) syncTray() {
	l := langOf(win.WindowLayout(win.ForegroundWindow()))
	if l == langUnknown {
		l = a.lang
		if l == langUnknown {
			l = langEN
		}
	}
	if l == a.lang && a.tray != nil {
		return
	}
	a.lang = l
	if a.tray != nil {
		a.tray.SetIcon(a.iconFor(l))
		a.tray.SetTip(a.tip(l))
	}
}

// postSync asks the message loop to refresh the badge (safe from any goroutine).
func (a *App) postSync() { win.PostMessage(a.hwnd, umSync, 0, 0) }

// promoteTrayIcon pins the icon to the taskbar on Windows 11, where new icons
// otherwise disappear into the overflow flyout.
func (a *App) promoteTrayIcon() {
	if a.promoteTries >= 5 {
		return
	}
	a.promoteTries++
	if win.PromoteTrayIcon(win.ExePath()) {
		a.promoteTries = 5
		if a.tray != nil {
			a.tray.Add()
			a.syncTray()
		}
		logf("tray: icon promoted to the taskbar")
	}
}

func (a *App) balloon(title, text string, flags uint32) {
	if !a.cfg.Notifications || a.tray == nil {
		return
	}
	a.tray.Balloon(title, text, flags)
}

func (a *App) showMenu() {
	m := win.NewPopupMenu()
	defer m.Destroy()

	if a.lang == langUnknown {
		m.AddInfo("Сейчас: язык не определён")
	} else {
		m.AddInfo("Сейчас: " + a.lang.String())
	}
	m.AddInfo("Левый Shift → English")
	m.AddInfo("Правый Shift → Русский")
	m.Separator()
	m.Add(cmdAutostart, "Запускать при входе в Windows", a.cfg.Autostart)
	m.Add(cmdAdmin, "Запуск с правами администратора", a.cfg.AdminMode)
	m.Separator()
	a.mu.Lock()
	ready, ver := a.updatePath, a.updateVersion
	a.mu.Unlock()
	if ready != "" {
		m.Add(cmdCheck, "Установить обновление "+ver, false)
	} else {
		m.Add(cmdCheck, "Проверить обновления", false)
	}
	m.Add(cmdInstall, "Открыть папку программы", false)
	m.Add(cmdFolder, "Открыть лог", false)
	m.Add(cmdAbout, "О программе ("+version+")", false)
	m.Separator()
	m.Default(cmdAbout)
	m.Add(cmdExit, "Выход", false)

	a.handleCommand(m.Track(a.hwnd))
}

func (a *App) handleCommand(id uint32) {
	switch id {
	case cmdAutostart:
		a.cfg.Autostart = !a.cfg.Autostart
		if err := a.cfg.save(); err != nil {
			logf("config: save failed: %v", err)
		}
		a.syncAutostart()
		if a.cfg.Autostart {
			a.balloon("Автозапуск включён", a.autostartDescription(), win.NIIF_INFO)
		} else {
			a.balloon("Автозапуск выключен", "Приложение больше не будет стартовать само.", win.NIIF_INFO)
		}

	case cmdAdmin:
		if a.cfg.AdminMode && win.IsElevated() {
			// turning admin mode off: fall back to the plain Run key
			a.cfg.AdminMode = false
		} else {
			a.cfg.AdminMode = true
		}
		if err := a.cfg.save(); err != nil {
			logf("config: save failed: %v", err)
		}
		if a.cfg.AdminMode && !win.IsElevated() {
			a.balloon("Нужен перезапуск",
				"Приложение перезапустится с правами администратора, чтобы работать в программах, запущенных от администратора.",
				win.NIIF_INFO)
			go func() {
				time.Sleep(400 * time.Millisecond)
				exe := win.ExePath()
				if exe == "" || !win.ShellExecute("runas", exe, "", filepath.Dir(exe), win.SW_SHOWNORMAL) {
					logf("admin mode: elevation cancelled")
				}
				win.PostMessage(a.hwnd, win.WM_CLOSE, 0, 0)
			}()
			return
		}
		a.syncAutostart()
		if a.cfg.AdminMode {
			a.balloon("Права администратора включены",
				"Работает до перезапуска. Для следующего входа настроена задача планировщика.", win.NIIF_INFO)
		} else {
			a.balloon("Права администратора выключены",
				"Приложения, запущенные от администратора, снова не будут перехватываться. Перезапустите программу.",
				win.NIIF_INFO)
		}

	case cmdCheck:
		a.mu.Lock()
		ready := a.updatePath
		a.mu.Unlock()
		if ready != "" {
			if err := a.applyUpdate(); err != nil {
				a.balloon("Обновление", err.Error(), win.NIIF_WARNING)
			}
			return
		}
		a.checkUpdates(true)

	case cmdInstall:
		dir := filepath.Dir(win.ExePath())
		win.ShellExecute("open", dir, "", "", win.SW_SHOWNORMAL)

	case cmdFolder:
		win.ShellExecute("open", logPath(), "", "", win.SW_SHOWNORMAL)

	case cmdAbout:
		win.MessageBox(a.aboutText(), appName+" "+version, win.MB_OK|win.MB_ICONINFORMATION)

	case cmdExit:
		win.PostMessage(a.hwnd, win.WM_CLOSE, 0, 0)
	}
}

func (a *App) aboutText() string {
	priv := "обычные права"
	if win.IsElevated() {
		priv = "права администратора"
	}
	mode := "по отпусканию (не мешает Shift+буква)"
	if a.cfg.Mode == "down" {
		mode = "по нажатию"
	}
	return fmt.Sprintf(`%s %s
Yet another language switcher

Левый Shift  → English
Правый Shift → Русский

Переключение: %s
Права: %s
Автозапуск: %s
Обновления: %s

Конфиг: %s
Лог: %s

%s`,
		appName, version, mode, priv, a.autostartDescription(),
		autoUpdateText(a.cfg), configPath(), logPath(), repoURL)
}

func autoUpdateText(cfg Config) string {
	if !cfg.AutoUpdate {
		return "выключены"
	}
	return fmt.Sprintf("проверка каждые %d ч", cfg.UpdateCheckHours)
}

func helpText() string {
	return appName + ` ` + version + `
Yet another language switcher.

Левый Shift  — переключить на English
Правый Shift — переключить на Русский

Приложение живёт в трее: меню открывается левым или правым кликом по иконке.
Иконка показывает текущий язык: синяя «A» — English, красная «Я» — Русский.

Флаги:
  --version   показать версию
  --help      это сообщение

Конфиг: ` + configPath() + `
Лог:    ` + logPath()
}
