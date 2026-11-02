//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"

	"github.com/dvygolov/yalangswitcher/internal/win"
)

// Autostart has two flavours:
//
//   - adminMode (default): a visible HKCU\...\Run entry starts the exe with
//     "--start-task", and that mode only asks Task Scheduler to run the real
//     task. The task carries "highest privileges", so the tray app comes up
//     elevated without a UAC prompt and stays visible in the Windows
//     "Startup apps" list, where it can be switched off like any other app.
//   - plain: the Run entry starts the exe directly, no elevation at all.
//
// Without elevation the keyboard hook cannot see keys typed into elevated
// windows, which is the whole point of adminMode.

const createNoWindow = 0x08000000

// schtasks reports "the task is already running" as a positive code, not as a
// failure - it must not send us into the UAC fallback.
const taskAlreadyRunning = 267009

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

// startLogonTask asks Task Scheduler to start the elevated instance. It is the
// only thing "--start-task" does; the traceable autostart entry is the Run key.
func startLogonTask() error {
	out, err := runHidden(schtasksPath(), "/Run", "/TN", taskName)
	if err == nil {
		return nil
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == taskAlreadyRunning {
		return nil
	}
	msg := strings.TrimSpace(out)
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("schtasks /Run: %s", msg)
}

// createLogonTask (re)registers the elevated launcher. The task has no trigger
// of its own: it is started on demand by the Run entry, so disabling that entry
// really disables the autostart.
func createLogonTask(exe string) error {
	path := filepath.Join(os.TempDir(), "yalangswitcher-task.xml")
	if err := os.WriteFile(path, taskXML(exe), 0o644); err != nil {
		return err
	}
	defer os.Remove(path)
	out, err := runHidden(schtasksPath(), "/Create", "/F", "/TN", taskName, "/XML", path)
	if err != nil {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("schtasks /Create: %s", msg)
	}
	return nil
}

func deleteLogonTask() {
	if _, err := runHidden(schtasksPath(), "/Delete", "/F", "/TN", taskName); err != nil {
		logf("autostart: deleting the scheduled task failed: %v", err)
	}
}

// taskXML mirrors what Task Scheduler itself exports for such a task: highest
// privileges, no trigger, no execution time limit (the default 72 hours would
// kill a tray app after three days), one instance at a time.
func taskXML(exe string) []byte {
	user := os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	body := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Author>YALangSwitcher</Author>
    <Description>Starts YALangSwitcher with administrator rights without a UAC prompt.</Description>
    <URI>\` + taskName + `</URI>
  </RegistrationInfo>
  <Principals>
    <Principal id="Author">
      <UserId>` + xmlEscape(user) + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
  </Settings>
  <Triggers />
  <Actions Context="Author">
    <Exec>
      <Command>` + xmlEscape(exe) + `</Command>
    </Exec>
  </Actions>
</Task>
`
	// schtasks only accepts Unicode XML.
	units := utf16.Encode([]rune(body))
	out := make([]byte, 0, 2+len(units)*2)
	out = append(out, 0xFF, 0xFE)
	for _, u := range units {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}

func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// syncAutostart makes the system state match the config.
func (a *App) syncAutostart() {
	exe := win.ExePath()
	elevated := win.IsElevated()

	if !a.cfg.Autostart {
		win.DeleteRunValue(appName)
		if elevated {
			deleteLogonTask()
		}
		logf("autostart: disabled")
		return
	}

	if !a.cfg.AdminMode {
		win.SetRunValue(appName, exe, "")
		if elevated {
			deleteLogonTask()
		}
		logf("autostart: HKCU Run entry set for %s (no admin rights)", exe)
		return
	}

	// adminMode: the Run entry is the starter, the task does the elevation.
	if !win.SetRunValue(appName, exe, "--start-task") {
		logf("autostart: writing the HKCU Run entry failed")
	}
	if !elevated {
		// the task can only be created elevated; the entry above will start
		// the existing one, or fall back to a UAC prompt
		logf("autostart: admin mode needs elevation, task left as is")
		return
	}
	if taskExists() && strings.EqualFold(a.cfg.InstalledExe, exe) {
		return
	}
	if err := createLogonTask(exe); err != nil {
		logf("autostart: creating the scheduled task failed: %v", err)
		a.balloon("Автозапуск", "Не удалось создать задачу планировщика, будет запрос UAC при входе.", win.NIIF_WARNING)
		return
	}
	logf("autostart: scheduled task created for %s", exe)
	a.cfg.InstalledExe = exe
	if err := a.cfg.save(); err != nil {
		logf("config: save failed: %v", err)
	}
}

// autostartDescription is the human readable state shown in the About box.
func (a *App) autostartDescription() string {
	if !a.cfg.Autostart {
		return "выключен"
	}
	if !a.cfg.AdminMode {
		if win.RunValue(appName) != "" {
			return "ключ автозапуска в реестре (без прав администратора)"
		}
		return "не настроен"
	}
	if taskExists() && win.RunValue(appName) != "" {
		return "автозагрузка + задача планировщика (без UAC)"
	}
	return "не настроен"
}
