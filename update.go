//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dvygolov/yalangswitcher/internal/win"
)

// Self-update: ask the GitHub API for the newest release, download the exe and
// swap the running file (rename + copy), then relaunch. Stdlib only.
const (
	updateRepo  = "dvygolov/YALangSwitcher"
	updateAPI   = "https://api.github.com/repos/" + updateRepo + "/releases/latest"
	updateAsset = "YALangSwitcher.exe"
)

type ghRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

func (a *App) rearmUpdateTimer() {
	if a.cfg.AutoUpdate {
		hours := time.Duration(a.cfg.UpdateCheckHours) * time.Hour
		win.SetTimer(a.hwnd, timerUpdate, uint32(hours/time.Millisecond))
	}
}

func (a *App) checkUpdates(manual bool) {
	a.mu.Lock()
	if a.updateBusy {
		a.mu.Unlock()
		return
	}
	a.updateBusy = true
	a.updateManual = manual
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.updateBusy = false
			a.mu.Unlock()
		}()
		a.mu.Lock()
		a.updateMsg = ""
		a.mu.Unlock()

		rel, err := fetchLatestRelease()
		if err != nil {
			logf("update: check failed: %v", err)
			a.failUpdate("Не удалось проверить обновления: " + err.Error())
			return
		}
		newer, comparable := versionLess(version, rel.TagName)
		if !comparable || !newer {
			logf("update: already up to date (%s)", version)
			a.failUpdate("Установлена последняя версия " + version)
			return
		}
		assetURL, assetSize := "", int64(0)
		for _, as := range rel.Assets {
			if strings.EqualFold(as.Name, updateAsset) {
				assetURL, assetSize = as.URL, as.Size
				break
			}
		}
		if assetURL == "" {
			logf("update: release %s has no %s asset", rel.TagName, updateAsset)
			a.failUpdate("В релизе " + rel.TagName + " нет файла " + updateAsset)
			return
		}
		logf("update: %s available, downloading %s", rel.TagName, assetURL)
		path, err := downloadAsset(assetURL, assetSize)
		if err != nil {
			logf("update: download failed: %v", err)
			a.failUpdate("Не удалось скачать обновление: " + err.Error())
			return
		}
		a.mu.Lock()
		a.updateVersion = strings.TrimPrefix(rel.TagName, "v")
		a.updatePath = path
		a.mu.Unlock()
		win.PostMessage(a.hwnd, umUpdateReady, 0, 0)
	}()
}

func (a *App) failUpdate(msg string) {
	a.mu.Lock()
	a.updateMsg = msg
	a.mu.Unlock()
	win.PostMessage(a.hwnd, umUpdateFailed, 0, 0)
}

func fetchLatestRelease() (*ghRelease, error) {
	req, err := http.NewRequest("GET", updateAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", appName+"/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, errors.New("релизов пока нет")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub ответил %s", resp.Status)
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, errors.New("пустой ответ GitHub")
	}
	return &rel, nil
}

func downloadAsset(url string, size int64) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", appName+"/"+version)
	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("скачивание: %s", resp.Status)
	}
	dir := filepath.Join(os.TempDir(), "YALangSwitcher-update")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, updateAsset)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, 32<<20))
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if size > 0 && n != size {
		return "", fmt.Errorf("получено %d байт вместо %d", n, size)
	}
	head := make([]byte, 2)
	fh, err := os.Open(path)
	if err != nil {
		return "", err
	}
	_, readErr := io.ReadFull(fh, head)
	_ = fh.Close()
	if readErr != nil || head[0] != 'M' || head[1] != 'Z' {
		return "", errors.New("скачанный файл не является exe")
	}
	return path, nil
}

// applyUpdate swaps the running executable and restarts the app.
func (a *App) applyUpdate() error {
	a.mu.Lock()
	path, ver := a.updatePath, a.updateVersion
	a.mu.Unlock()
	if path == "" {
		return errors.New("обновление ещё не скачано")
	}
	cur := win.ExePath()
	if cur == "" {
		return errors.New("не удалось определить путь exe")
	}
	old := cur + ".old"
	_ = os.Remove(old)
	if err := os.Rename(cur, old); err != nil {
		return fmt.Errorf("файл занят (%v) — обновитесь вручную", err)
	}
	if err := copyFile(path, cur); err != nil {
		_ = os.Rename(old, cur)
		return err
	}
	_ = os.Remove(path)

	a.mu.Lock()
	a.updatePath = ""
	a.mu.Unlock()

	// hand the single instance slot over before the new process starts
	if a.instanceMutex != 0 {
		win.CloseHandle(a.instanceMutex)
		a.instanceMutex = 0
	}
	if !win.ShellExecute("open", cur, "", filepath.Dir(cur), win.SW_SHOWNORMAL) {
		logf("update: relaunch failed, restart manually")
		a.balloon("Обновление "+ver, "Файл обновлён, перезапустите приложение вручную.", win.NIIF_INFO)
		return nil
	}
	logf("update: installed %s, relaunching", ver)
	a.balloon("Обновление "+ver, "Приложение перезапускается.", win.NIIF_INFO)
	win.PostMessage(a.hwnd, win.WM_CLOSE, 0, 0)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// cleanupOldBinary removes the leftover of a previous self-update.
func cleanupOldBinary() {
	cur := win.ExePath()
	if cur == "" {
		return
	}
	if _, err := os.Stat(cur + ".old"); err == nil {
		if err := os.Remove(cur + ".old"); err == nil {
			logf("update: previous binary removed")
		}
	}
}

// versionLess reports whether a is older than b. ok is false when either tag is
// not a plain numeric version, so dev builds never downgrade.
func versionLess(a, b string) (less bool, ok bool) {
	pa, okA := parseVersion(a)
	pb, okB := parseVersion(b)
	if !okA || !okB {
		return false, false
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i], true
		}
	}
	return false, true
}

func parseVersion(s string) ([3]int, bool) {
	var out [3]int
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "v"))
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return out, false
	}
	parts := strings.Split(s, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
