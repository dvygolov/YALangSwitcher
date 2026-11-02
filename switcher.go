//go:build windows

package main

import (
	"time"

	"github.com/dvygolov/yalangswitcher/internal/win"
)

// Lang is the language a Shift key is bound to.
type Lang int

const (
	langUnknown Lang = iota
	langEN
	langRU
)

func (l Lang) String() string {
	switch l {
	case langEN:
		return "English"
	case langRU:
		return "Русский"
	}
	return "неизвестно"
}

func (l Lang) short() string {
	switch l {
	case langEN:
		return "en"
	case langRU:
		return "ru"
	}
	return "??"
}

// primary is the Windows primary language id: en-US, en-GB and friends all
// share LANG_ENGLISH, ru-RU and ru-UA share LANG_RUSSIAN.
func (l Lang) primary() uint32 {
	if l == langRU {
		return win.LANG_RUSSIAN
	}
	return win.LANG_ENGLISH
}

func (l Lang) klid(cfg *Config) string {
	if l == langRU {
		return cfg.RussianLayout
	}
	return cfg.EnglishLayout
}

// langOf maps a keyboard layout handle to a Lang.
func langOf(hkl uintptr) Lang {
	switch win.PrimaryLang(hkl) {
	case win.LANG_ENGLISH:
		return langEN
	case win.LANG_RUSSIAN:
		return langRU
	}
	return langUnknown
}

// shiftState tracks one Shift key: whether it is held and whether any other key
// was pressed while it was down (a real Shift+key combo, not a Shift tap).
type shiftState struct {
	down  bool
	dirty bool
}

// Switcher turns Shift taps into layout switch requests.
//
// Default mode is "tap": the layout changes when Shift is released and nothing
// else was pressed in between, so Shift+letter keeps working for capitals.
// Mode "down" switches immediately on press, exactly as the key goes down.
type Switcher struct {
	cfg  *Config
	emit func(Lang)

	left  shiftState
	right shiftState
}

func shiftSide(e win.KeyEvent) Lang {
	switch e.VK {
	case win.VK_LSHIFT:
		return langEN
	case win.VK_RSHIFT:
		return langRU
	case win.VK_SHIFT:
		switch e.Scan {
		case win.KBD_SCAN_LSHIFT:
			return langEN
		case win.KBD_SCAN_RSHIFT:
			return langRU
		}
	}
	return langUnknown
}

// handle is called from the low level keyboard hook, for every key of the
// system: it must stay allocation free and never block.
func (s *Switcher) handle(e win.KeyEvent) {
	if e.Ours {
		return
	}
	if e.Injected && !s.cfg.AcceptInjected {
		return
	}

	side := shiftSide(e)
	if side == langUnknown {
		if e.Down && (s.left.down || s.right.down) {
			if s.left.down {
				s.left.dirty = true
			}
			if s.right.down {
				s.right.dirty = true
			}
		}
		return
	}

	st, other := &s.left, &s.right
	if side == langRU {
		st, other = &s.right, &s.left
	}

	if e.Down {
		fresh := !st.down
		if fresh {
			st.down = true
			st.dirty = other.down // pressing the other Shift makes it a combo
		}
		if fresh && s.cfg.Mode == "down" {
			s.emit(side)
		}
		return
	}

	ready := st.down && !st.dirty
	st.down, st.dirty = false, false
	if s.cfg.Mode == "down" || !ready {
		return
	}
	// Shift was only part of a modifier combination: leave the layout alone.
	if win.KeyDown(win.VK_CONTROL) || win.KeyDown(win.VK_MENU) ||
		win.KeyDown(win.VK_LWIN) || win.KeyDown(win.VK_RWIN) {
		return
	}
	// Shift+drag (text selection) is not a switch either.
	if win.KeyDown(win.VK_LBUTTON) || win.KeyDown(win.VK_RBUTTON) {
		return
	}
	s.emit(side)
}

// findLayout returns the installed layout for a language, loading the
// configured KLID when the user has no such layout yet.
func findLayout(l Lang, cfg *Config) uintptr {
	for _, hkl := range win.Layouts() {
		if win.PrimaryLang(hkl) == l.primary() {
			return hkl
		}
	}
	return win.LoadLayout(l.klid(cfg))
}

// activate switches the foreground application to a language.
func (a *App) activate(l Lang) {
	hkl := findLayout(l, &a.cfg)
	if hkl == 0 {
		logf("switch: no layout for %s (klid %s)", l, l.klid(&a.cfg))
		return
	}
	fg := win.ForegroundWindow()
	if fg == 0 || !win.IsWindow(fg) {
		logf("switch: no foreground window")
		return
	}
	if langOf(win.WindowLayout(fg)) == l {
		a.syncTray()
		return
	}
	if !win.PostLayoutChange(fg, hkl) {
		logf("switch: PostMessage(WM_INPUTLANGCHANGEREQUEST) failed")
	}
	logf("switch: %s -> %s (hkl %#x)", langOf(win.WindowLayout(fg)), l, hkl)
	go a.verifyLayout(l, hkl)
}

// verifyLayout checks that the foreground window really switched. Windows that
// do not pass WM_INPUTLANGCHANGEREQUEST to DefWindowProc (some games, some
// custom toolkits) ignore it; for those the language hotkey is replayed.
func (a *App) verifyLayout(l Lang, hkl uintptr) {
	time.Sleep(120 * time.Millisecond)
	if langOf(win.WindowLayout(win.ForegroundWindow())) == l {
		a.postSync()
		return
	}
	hotkey := a.cfg.FallbackHotkey
	if hotkey == "" || hotkey == "none" {
		logf("switch: %s did not switch and the fallback hotkey is disabled", l)
		return
	}
	steps := cycleSteps(win.WindowLayout(win.ForegroundWindow()), hkl)
	logf("switch: layout message ignored, replaying %s x%d", hotkey, steps)
	for i := 0; i < steps; i++ {
		if !win.SendHotkey(hotkey) {
			logf("switch: SendInput failed")
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	time.Sleep(120 * time.Millisecond)
	if langOf(win.WindowLayout(win.ForegroundWindow())) == l {
		logf("switch: %s reached via %s hotkey", l, hotkey)
		a.postSync()
		return
	}
	logf("switch: window ignored both the layout message and the %s hotkey", hotkey)
}

// cycleSteps counts how many hotkey presses it takes to walk from one layout to
// the target in the user's layout cycle.
func cycleSteps(cur, target uintptr) int {
	list := win.Layouts()
	if len(list) <= 1 {
		return 1
	}
	from, to := -1, -1
	for i, h := range list {
		if h == cur {
			from = i
		}
		if h == target {
			to = i
		}
	}
	if from < 0 || to < 0 {
		return 1
	}
	steps := (to - from + len(list)) % len(list)
	if steps == 0 {
		steps = len(list)
	}
	return steps
}
