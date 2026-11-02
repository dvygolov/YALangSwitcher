//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Tiny append-only log: a tray app has no console, so when the hook refuses to
// install or a layout cannot be switched this file is the only evidence.
const logMaxSize = 512 << 10

var (
	logMu   sync.Mutex
	logFile *os.File
	logOff  bool
)

func logPath() string { return filepath.Join(dataDir(), "yalangswitcher.log") }

func openLog() {
	if logFile != nil || logOff {
		return
	}
	if err := os.MkdirAll(dataDir(), 0o755); err != nil {
		logOff = true
		return
	}
	path := logPath()
	if st, err := os.Stat(path); err == nil && st.Size() > logMaxSize {
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logOff = true
		return
	}
	logFile = f
}

func logf(format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	openLog()
	if logFile == nil {
		return
	}
	line := fmt.Sprintf("%s %s", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
	_, _ = logFile.WriteString(line + "\r\n")
}

func closeLog() {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
}
