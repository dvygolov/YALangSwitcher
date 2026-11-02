//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/dvygolov/yalangswitcher/internal/win"
)

// Autostart has two flavours:
//
//   - adminMode: a Task Scheduler job with "run with highest privileges" that
//     starts the app silently at logon (a Run key cannot elevate without a UAC
//     prompt, and without elevation the keyboard hook cannot see keys typed
//     into elevated windows).
//   - plain: an HKCU\...\Run entry, no UAC at all.

const createNoWindow = 0x08000000

func runHidden(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func schtasksPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "schtasks.exe")
}

func taskExists() bool {
	_, err := runHidden(schtasksPath(), "/Query", "/TN", taskName)
	return err == nil
}

func createLogonTask(exe string) error {
	out, err := runHidden(schtasksPath(),
		"/Create", "/F", "/SC", "ONLOGON", "/RL", "HIGHEST",
		"/TN", taskName, "/TR", `"`+exe+`"`)
	if err != nil {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("schtasks: %s", msg)
	}
	return nil
}

func deleteLogonTask() {
	if _, err := runHidden(schtasksPath(), "/Delete", "/F", "/TN", taskName); err != nil {
		logf("autostart: deleting the scheduled task failed: %v", err)
	}
}

// syncAutostart makes the system state match the config.
func (a *App) syncAutostart() {
	exe := win.ExePath()
	elevated := win.IsElevated()

	switch {
	case !a.cfg.Autostart:
		win.DeleteRunValue(appName)
		if elevated {
			deleteLogonTask()
		}
		logf("autostart: disabled")

	case a.cfg.AdminMode && elevated:
		win.DeleteRunValue(appName)
		if taskExists() && strings.EqualFold(a.cfg.InstalledExe, exe) {
			return
		}
		if err := createLogonTask(exe); err != nil {
			logf("autostart: creating the scheduled task failed: %v", err)
			// never leave the user without autostart: the Run key still works,
			// it just cannot cover elevated windows
			win.SetRunValue(appName, exe)
			a.balloon("Автозапуск", "Не удалось создать задачу планировщика, включён обычный автозапуск.", win.NIIF_WARNING)
			return
		}
		logf("autostart: scheduled task created for %s", exe)
		a.cfg.InstalledExe = exe
		if err := a.cfg.save(); err != nil {
			logf("config: save failed: %v", err)
		}

	case a.cfg.AdminMode && !elevated:
		// cannot create or remove the task without elevation; keep whatever
		// is already there and do not touch the Run key
		logf("autostart: admin mode needs elevation, task left untouched")

	default:
		if elevated {
			deleteLogonTask()
		}
		if win.SetRunValue(appName, exe) {
			logf("autostart: HKCU Run entry set for %s", exe)
		} else {
			logf("autostart: writing the HKCU Run entry failed")
		}
	}
}

// autostartDescription is the human readable state shown in the menu and the
// About box.
func (a *App) autostartDescription() string {
	if !a.cfg.Autostart {
		return "выключен"
	}
	if a.cfg.AdminMode {
		if taskExists() {
			return "задача планировщика (с правами администратора)"
		}
		return "не настроен (задачи планировщика нет)"
	}
	if win.RunValue(appName) != "" {
		return "ключ автозапуска в реестре"
	}
	return "не настроен"
}
