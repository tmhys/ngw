//go:build windows

// Package gui is ngw's window: a Win32 window that shows Neovim's grid
// with GDI and takes keyboard (including the input method), mouse,
// clipboard and file drops.
package gui

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"ngw/internal/input"
	"ngw/internal/msgpack"
	"ngw/internal/nvim"
	"ngw/internal/screen"
	"ngw/internal/win32"
)

const (
	wmAppQueue = win32.WM_APP + 1 // Neovim sent something
	wmAppQuit  = win32.WM_APP + 2 // the connection ended
	wmAppFont  = win32.WM_APP + 3 // ":set guifont=*": show the font dialog

	timerShow = 1

	className = "ngwWindow"
	style     = win32.WS_OVERLAPPEDWINDOW
	exStyle   = win32.WS_EX_ACCEPTFILES
)

// Options are the command line choices that concern the window.
type Options struct {
	Cols, Rows int
	Maximized  bool
	// Attached is true for --server: closing the window only detaches.
	Attached bool
	// Connect starts or attaches to Neovim.
	Connect func(onNotify func(string, []any), onRequest func(string, []any) (any, error)) (*nvim.Nvim, error)
}

type item struct {
	method string
	params []any
	frame  [][]any // redraw batches up to and including a flush
}

// App is the running GUI.
type App struct {
	hwnd  uintptr
	nv    *nvim.Nvim
	scr   *screen.Screen
	fonts *Fonts
	r     *renderer
	dpi   uint32

	mu      sync.Mutex
	queue   []item
	pending [][]any // redraw batches since the last flush
	posted  bool

	focused   bool
	shown     bool
	uiEntered bool
	maximized bool
	title     string

	// what the grid should be; sent with nvim_ui_try_resize
	cols, rows int

	guifont, guifontwide string
	linespace            int
	ambiDouble           bool

	// input method
	imeInsertOnly  bool
	imeOffOnLeave  bool
	imeEnabled     bool
	imeWasOpen     bool
	imeFromCmdline bool
	imeForce       int8 // g:ngw_ime: 1 on, -1 off, 0 by mode
	pre            *preedit
	preRect        win32.RECT
	highSurrogate  uint16

	// mouse
	button             string
	mouseRow, mouseCol int
	wheelV, wheelH     int

	curRect    win32.RECT
	imePos     win32.POINT
	attached   bool
	fullscreen bool
	placement  win32.WINDOWPLACEMENT

	clipMu      sync.Mutex
	clipText    string
	clipRegtype string
}

var theApp *App

// Run creates the window, starts Neovim and runs until Neovim exits.
func Run(opts Options) error {
	runtime.LockOSThread()
	win32.SetDPIAware()

	a := &App{
		scr:           screen.New(max(opts.Cols, 1), max(opts.Rows, 1)),
		cols:          opts.Cols,
		rows:          opts.Rows,
		imeInsertOnly: true,
		imeEnabled:    true,
		maximized:     opts.Maximized,
		attached:      opts.Attached,
		title:         "ngw",
	}
	theApp = a

	inst := win32.GetModuleHandle()
	wc := win32.WNDCLASSEXW{
		Style:     win32.CS_HREDRAW | win32.CS_VREDRAW,
		WndProc:   syscall.NewCallback(wndProc),
		Instance:  inst,
		Icon:      win32.LoadIcon(inst, 1),
		Cursor:    win32.LoadCursor(win32.IDC_ARROW),
		ClassName: win32.Str(className),
	}
	if wc.Icon == 0 {
		wc.Icon = win32.LoadIcon(0, win32.IDI_APPLICATION)
	}
	wc.IconSm = wc.Icon
	if win32.RegisterClassEx(&wc) == 0 {
		return fmt.Errorf("RegisterClassEx failed")
	}
	cw := int32(win32.CW_USEDEFAULT)
	a.hwnd = win32.CreateWindowEx(exStyle, className, a.title, style, cw, cw, 800, 600, inst)
	if a.hwnd == 0 {
		return fmt.Errorf("CreateWindowEx failed")
	}
	a.dpi = win32.GetDpiForWindow(a.hwnd)
	a.fonts = NewFonts("", "", 0, a.dpi)
	a.r = newRenderer(a.hwnd, a.scr, a.fonts)
	a.fitWindowToGrid()
	win32.DragAcceptFiles(a.hwnd, true)

	nv, err := opts.Connect(a.onNotify, a.onRequest)
	if err != nil {
		return err
	}
	a.nv = nv
	go func() {
		nv.Serve()
		win32.PostMessage(a.hwnd, wmAppQuit, 0, 0)
	}()
	if err := nv.Prepare(); err != nil {
		return fmt.Errorf("nvim: %w", err)
	}
	if err := nv.Attach(a.cols, a.rows); err != nil {
		return fmt.Errorf("nvim_ui_attach: %w", err)
	}
	// Show the window when Neovim has started (UIEnter), so that the
	// config's font and colors are there from the first frame; but never
	// wait longer than this.
	win32.SetTimer(a.hwnd, timerShow, 1500)

	var msg win32.MSG
	for win32.GetMessage(&msg) > 0 {
		if (msg.Message == win32.WM_KEYDOWN || msg.Message == win32.WM_SYSKEYDOWN) && msg.Hwnd == a.hwnd {
			if a.keyDown(&msg) {
				continue // not translated: no WM_CHAR follows
			}
		}
		win32.TranslateMessage(&msg)
		win32.DispatchMessage(&msg)
	}
	writeStats()
	return nil
}

// onNotify runs on the RPC reader goroutine. It queues what Neovim sent
// and wakes the UI thread; redraw batches are queued only once a flush
// completes them, so the UI never draws a half-updated screen.
func (a *App) onNotify(method string, params []any) {
	a.mu.Lock()
	if method == "redraw" {
		a.pending = append(a.pending, params)
		if !hasFlush(params) {
			a.mu.Unlock()
			return
		}
		a.queue = append(a.queue, item{method: method, frame: a.pending})
		a.pending = nil
	} else {
		a.queue = append(a.queue, item{method: method, params: params})
	}
	post := !a.posted
	a.posted = true
	a.mu.Unlock()
	if post {
		win32.PostMessage(a.hwnd, wmAppQueue, 0, 0)
	}
}

func hasFlush(params []any) bool {
	for i := len(params) - 1; i >= 0; i-- {
		if ev, ok := params[i].([]any); ok && len(ev) > 0 {
			if name, _ := ev[0].(string); name == "flush" {
				return true
			}
		}
	}
	return false
}

// onRequest answers rpcrequest() from Neovim: the clipboard provider.
func (a *App) onRequest(method string, params []any) (any, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	switch method {
	case nvim.ClipGetRequest:
		text, ok := win32.ClipboardText(a.hwnd)
		if !ok {
			return nil, fmt.Errorf("cannot open the clipboard")
		}
		text = strings.ReplaceAll(text, "\r\n", "\n")
		a.clipMu.Lock()
		regtype := "v"
		if text == a.clipText {
			regtype = a.clipRegtype
		} else if strings.HasSuffix(text, "\n") {
			regtype = "V"
		}
		a.clipMu.Unlock()
		if regtype == "V" {
			text = strings.TrimSuffix(text, "\n")
		}
		lines := strings.Split(text, "\n")
		out := make([]any, len(lines))
		for i, l := range lines {
			out[i] = l
		}
		return []any{out, regtype}, nil
	case nvim.ClipSetRequest:
		var lines []string
		if len(params) > 0 {
			l, _ := params[0].([]any)
			for _, v := range l {
				s, _ := v.(string)
				lines = append(lines, s)
			}
		}
		regtype := "v"
		if len(params) > 1 {
			regtype, _ = params[1].(string)
		}
		text := strings.Join(lines, "\n")
		if regtype == "V" {
			text += "\n"
		}
		a.clipMu.Lock()
		a.clipText, a.clipRegtype = text, regtype
		a.clipMu.Unlock()
		if !win32.SetClipboardText(a.hwnd, strings.ReplaceAll(text, "\n", "\r\n")) {
			return nil, fmt.Errorf("cannot set the clipboard")
		}
		return nil, nil
	}
	return nil, fmt.Errorf("ngw: unknown request %s", method)
}

// process applies what Neovim sent and draws it.
func (a *App) process() {
	a.mu.Lock()
	items := a.queue
	a.queue = nil
	a.posted = false
	a.mu.Unlock()

	oldCursor := a.r.cursorRect()
	drew := false
	for _, it := range items {
		switch it.method {
		case "redraw":
			var t0 time.Time
			if st.on {
				t0 = time.Now()
				st.Frames++
				st.Batches += len(it.frame)
			}
			for _, batch := range it.frame {
				if _, err := a.scr.HandleRedraw(batch); err != nil {
					continue
				}
			}
			if st.on {
				st.Apply += since(t0)
			}
			a.applyOptions()
			drew = true
		case nvim.UIEnterNotification:
			a.uiEntered = true
		case nvim.SetSizeNotification:
			if len(it.params) >= 2 {
				c, _ := msgpack.ToInt(it.params[0])
				l, _ := msgpack.ToInt(it.params[1])
				a.cols, a.rows = int(c), int(l)
				if !a.maximized && !a.fullscreen {
					a.fitWindowToGrid()
				}
			}
		case nvim.VarNotification:
			a.setVar(it.params)
		}
	}
	if !drew {
		return
	}
	var t0 time.Time
	if st.on {
		t0 = time.Now()
	}
	a.r.drawDirty()
	a.r.flushInvalid()
	if st.on {
		st.Draw += since(t0)
	}
	if a.scr.TakeColorsChanged() {
		_, _, b := colorParts(a.scr.DefaultBG)
		win32.SetDarkTitleBar(a.hwnd, b < 128)
	}
	if a.scr.Title != "" && a.scr.Title != a.title {
		a.title = a.scr.Title
		win32.SetWindowText(a.hwnd, a.title)
	}
	nc := a.r.cursorRect()
	if nc != oldCursor || a.curRect != nc {
		win32.InvalidateRect(a.hwnd, &oldCursor)
		win32.InvalidateRect(a.hwnd, &a.curRect)
		win32.InvalidateRect(a.hwnd, &nc)
	} else {
		win32.InvalidateRect(a.hwnd, &nc)
	}
	a.curRect = nc
	a.updateIME()
	if a.uiEntered && !a.shown {
		a.show()
	}
}

func colorParts(c screen.Color) (r, g, lum int) {
	rr, gg, bb := c.RGB()
	return int(rr), int(gg), (int(rr)*299 + int(gg)*587 + int(bb)*114) / 1000
}

func (a *App) show() {
	if a.shown {
		return
	}
	a.shown = true
	win32.KillTimer(a.hwnd, timerShow)
	if a.maximized {
		win32.ShowWindow(a.hwnd, win32.SW_SHOWMAXIMIZED)
	} else {
		win32.ShowWindow(a.hwnd, win32.SW_SHOWNORMAL)
	}
	win32.UpdateWindow(a.hwnd)
	win32.SetForegroundWindow(a.hwnd)
}

// applyOptions reacts to UI options Neovim set.
func (a *App) applyOptions() {
	fontsChanged := false
	for _, name := range a.scr.TakeOptionChanges() {
		switch name {
		case "guifont":
			v := a.scr.OptionString("guifont")
			if strings.TrimSpace(v) == "*" {
				// Not from here: the dialog's modal loop would process
				// more frames in the middle of this one.
				win32.PostMessage(a.hwnd, wmAppFont, 0, 0)
				continue
			}
			if v != a.guifont {
				a.guifont, fontsChanged = v, true
			}
		case "guifontwide":
			if v := a.scr.OptionString("guifontwide"); v != a.guifontwide {
				a.guifontwide, fontsChanged = v, true
			}
		case "linespace":
			if v := a.scr.OptionInt("linespace"); v != a.linespace {
				a.linespace, fontsChanged = v, true
			}
		case "ambiwidth":
			a.ambiDouble = a.scr.OptionString("ambiwidth") == "double"
		}
	}
	if fontsChanged {
		a.setFonts()
	}
}

// setFonts recreates the fonts. Before Neovim has started (the config is
// still running) the window follows the font, keeping the grid size, as
// GVim does; afterwards the window keeps its size and the grid follows.
func (a *App) setFonts() {
	changed := a.fonts.Set(a.guifont, a.guifontwide, a.linespace, a.dpi)
	a.r.font = 0
	a.imePos = win32.POINT{X: -1}
	a.scr.MarkAllDirty()
	if !changed {
		return
	}
	if !a.shown && !a.maximized {
		a.cols, a.rows = a.scr.Width, a.scr.Height
		a.fitWindowToGrid()
	} else {
		a.resizeGrid()
	}
}

func (a *App) chooseFont() {
	lf := a.fonts.LogFont
	pt, ok := win32.ChooseFont(a.hwnd, &lf)
	name := syscall.UTF16ToString(lf.FaceName[:])
	if !ok || name == "" {
		// Cancelled: put the old value back.
		a.nv.ExecLuaAsync("vim.o.guifont = ...", a.guifont)
		return
	}
	v := GuifontValue(name, float64(pt)/10)
	a.nv.ExecLuaAsync("vim.o.guifont = ...", v)
}

func (a *App) setVar(params []any) {
	if len(params) < 2 {
		return
	}
	name, _ := params[0].(string)
	truthy := func(v any) bool {
		switch x := v.(type) {
		case bool:
			return x
		case nil:
			return false
		}
		n, err := msgpack.ToInt(v)
		return err == nil && n != 0
	}
	switch name {
	case "ngw_ime_insert_only":
		a.imeInsertOnly = params[1] == nil || truthy(params[1])
		a.updateIME()
	case "ngw_ime":
		// Set from autocmds to decide exactly when the input method is
		// usable (like Neovide's g:neovide_input_ime); unset: by mode.
		switch {
		case params[1] == nil:
			a.imeForce = 0
		case truthy(params[1]):
			a.imeForce = 1
		default:
			a.imeForce = -1
		}
		a.updateIME()
	case "ngw_ime_off_on_leave":
		a.imeOffOnLeave = truthy(params[1])
	case "ngw_fullscreen":
		a.setFullscreen(truthy(params[1]))
	}
}

// fitWindowToGrid sizes the window so its client area holds a.cols ×
// a.rows cells.
func (a *App) fitWindowToGrid() {
	rc := win32.RECT{Right: int32(a.cols) * a.fonts.CellW, Bottom: int32(a.rows) * a.fonts.CellH}
	win32.AdjustWindowRect(&rc, style, exStyle, a.dpi)
	w, h := rc.Right-rc.Left, rc.Bottom-rc.Top
	_, work := win32.MonitorWorkArea(a.hwnd)
	w = min(w, work.Right-work.Left)
	h = min(h, work.Bottom-work.Top)
	win32.SetWindowPos(a.hwnd, 0, 0, 0, w, h, win32.SWP_NOMOVE|win32.SWP_NOZORDER|win32.SWP_NOACTIVATE)
}

// resizeGrid asks Neovim for the grid that fits the client area.
func (a *App) resizeGrid() {
	rc := win32.GetClientRect(a.hwnd)
	w, h := rc.Right-rc.Left, rc.Bottom-rc.Top
	if w <= 0 || h <= 0 {
		return
	}
	a.r.ensure(w, h)
	cols := max(1, int(w/a.fonts.CellW))
	rows := max(1, int(h/a.fonts.CellH))
	if cols != a.cols || rows != a.rows || cols != a.scr.Width || rows != a.scr.Height {
		a.cols, a.rows = cols, rows
		if a.nv != nil {
			a.nv.TryResize(cols, rows)
		}
	}
}

func (a *App) setFullscreen(on bool) {
	if on == a.fullscreen {
		return
	}
	a.fullscreen = on
	if on {
		win32.GetWindowPlacement(a.hwnd, &a.placement)
		mon, _ := win32.MonitorWorkArea(a.hwnd)
		win32.SetWindowLongPtr(a.hwnd, win32.GWL_STYLE, win32.WS_POPUP|win32.WS_VISIBLE)
		win32.SetWindowPos(a.hwnd, 0, mon.Left, mon.Top, mon.Right-mon.Left, mon.Bottom-mon.Top,
			win32.SWP_NOZORDER|win32.SWP_FRAMECHANGED)
	} else {
		win32.SetWindowLongPtr(a.hwnd, win32.GWL_STYLE, style|win32.WS_VISIBLE)
		win32.SetWindowPlacement(a.hwnd, &a.placement)
		win32.SetWindowPos(a.hwnd, 0, 0, 0, 0, 0,
			win32.SWP_NOMOVE|win32.SWP_NOSIZE|win32.SWP_NOZORDER|win32.SWP_FRAMECHANGED)
	}
}

func wndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	a := theApp
	if a == nil || a.hwnd == 0 || a.r == nil {
		return win32.DefWindowProc(hwnd, msg, wp, lp)
	}
	switch msg {
	case wmAppQueue:
		a.process()
		return 0
	case wmAppQuit:
		win32.DestroyWindow(hwnd)
		return 0
	case wmAppFont:
		a.chooseFont()
		return 0
	case win32.WM_TIMER:
		if wp == timerShow {
			a.show()
		}
		return 0
	case win32.WM_PAINT:
		a.paint()
		return 0
	case win32.WM_ERASEBKGND:
		return 1
	case win32.WM_SIZE:
		if wp != 1 { // not minimized
			a.maximized = wp == 2
			a.resizeGrid()
			// Draw everything again from the cells, margins included,
			// without waiting for Neovim: nothing drawn for another size
			// can be left in the bitmap.
			a.scr.MarkAllDirty()
			a.r.drawDirty()
			a.r.flushInvalid()
		}
		return 0
	case win32.WM_DPICHANGED:
		a.dpi = uint32(win32.LOWORD(wp))
		a.fonts.Set(a.guifont, a.guifontwide, a.linespace, a.dpi)
		a.r.font = 0
		a.scr.MarkAllDirty()
		rc := (*win32.RECT)(unsafe.Pointer(lp))
		win32.SetWindowPos(hwnd, 0, rc.Left, rc.Top, rc.Right-rc.Left, rc.Bottom-rc.Top, win32.SWP_NOZORDER|win32.SWP_NOACTIVATE)
		a.resizeGrid()
		a.r.drawDirty()
		win32.InvalidateRect(hwnd, nil)
		return 0
	case win32.WM_SETFOCUS:
		a.focused = true
		a.nv.SetFocus(true)
		a.updateIME()
		win32.InvalidateRect(hwnd, &a.curRect)
		return 0
	case win32.WM_KILLFOCUS:
		a.focused = false
		a.nv.SetFocus(false)
		win32.InvalidateRect(hwnd, &a.curRect)
		return 0
	case win32.WM_CLOSE:
		select {
		case <-a.nv.Done():
			win32.DestroyWindow(hwnd)
			return 0
		default:
		}
		if a.attached {
			// --server: leave the server running, as a terminal would.
			a.nv.Close()
			return 0
		}
		// Like closing GVim: Neovim asks about unsaved buffers.
		a.nv.Input("<C-\\><C-n>")
		a.nv.Command("confirm qall")
		return 0
	case win32.WM_DESTROY:
		win32.PostQuitMessage(0)
		return 0
	case win32.WM_CHAR:
		a.char(uint16(wp))
		return 0
	case win32.WM_SYSCHAR, win32.WM_DEADCHAR:
		return 0 // Alt chords are sent from WM_SYSKEYDOWN; no menu beep
	case win32.WM_SYSKEYDOWN:
		if wp == win32.VK_F4 && win32.KeyDown(win32.VK_MENU) {
			return win32.DefWindowProc(hwnd, msg, wp, lp) // Alt+F4 closes
		}
		return 0
	case win32.WM_SYSKEYUP:
		return 0 // Alt or F10 alone must not open the window menu
	case win32.WM_IME_SETCONTEXT:
		// ngw draws the composition itself, at the cursor.
		return win32.DefWindowProc(hwnd, msg, wp, lp&^win32.ISC_SHOWUICOMPOSITIONWINDOW)
	case win32.WM_IME_STARTCOMPOSITION:
		if c := win32.ImmGetContext(hwnd); c != 0 {
			lf := a.fonts.LogFont
			win32.ImmSetCompositionFont(c, &lf)
			win32.ImmReleaseContext(hwnd, c)
		}
		a.imePos = win32.POINT{X: -1}
		a.updateIME()
		return 0
	case win32.WM_IME_COMPOSITION:
		a.imeComposition(lp)
		return 0
	case win32.WM_IME_ENDCOMPOSITION:
		a.setPreedit(nil)
		return 0
	case win32.WM_IME_REQUEST:
		if wp == win32.IMR_QUERYCHARPOSITION {
			a.imeCharPosition((*win32.IMECHARPOSITION)(unsafe.Pointer(lp)))
			return 1
		}
	case win32.WM_LBUTTONDOWN, win32.WM_RBUTTONDOWN, win32.WM_MBUTTONDOWN:
		a.mouseButton(msg, wp, lp, "press")
		return 0
	case win32.WM_LBUTTONUP, win32.WM_RBUTTONUP, win32.WM_MBUTTONUP:
		a.mouseButton(msg, wp, lp, "release")
		return 0
	case win32.WM_MOUSEMOVE:
		a.mouseMove(wp, lp)
		return 0
	case win32.WM_MOUSEWHEEL, win32.WM_MOUSEHWHEEL:
		a.wheel(msg, wp, lp)
		return 0
	case win32.WM_CAPTURECHANGED:
		a.button = ""
		return 0
	case win32.WM_DROPFILES:
		files := win32.DroppedFiles(wp)
		list := make([]any, len(files))
		for i, f := range files {
			list[i] = f
		}
		a.nv.ExecLuaAsync(nvim.DropLua, list)
		win32.SetForegroundWindow(hwnd)
		return 0
	}
	return win32.DefWindowProc(hwnd, msg, wp, lp)
}

func (a *App) paint() {
	if st.on {
		t0 := time.Now()
		st.Paints++
		defer func() { st.Paint += since(t0) }()
	}
	var ps win32.PAINTSTRUCT
	dc := win32.BeginPaint(a.hwnd, &ps)
	rc := ps.RcPaint
	// Parts of the client area outside the bitmap (only while it is
	// being enlarged) get the default background.
	if rc.Right > a.r.bw || rc.Bottom > a.r.bh {
		out := rc
		win32.FillRect(dc, &out, colorref(a.scr.DefaultBG))
	}
	win32.BitBlt(dc, rc.Left, rc.Top, min(rc.Right, a.r.bw)-rc.Left, min(rc.Bottom, a.r.bh)-rc.Top, a.r.dc, rc.Left, rc.Top)
	win32.SetBkMode(dc, win32.TRANSPARENT)
	win32.SetTextAlign(dc, win32.TA_BASELINE|win32.TA_LEFT)
	a.r.font = 0
	if a.pre == nil || a.pre.text == "" {
		a.r.drawCursor(dc, a.focused)
	} else {
		cells := a.r.layoutPreedit(a.pre, a.ambiDouble)
		a.r.drawPreedit(dc, cells, a.pre.cursor)
	}
	a.r.font = 0
	win32.EndPaint(a.hwnd, &ps)
}

// Keyboard

func mods() input.Mod {
	var m input.Mod
	if win32.KeyDown(win32.VK_SHIFT) {
		m |= input.ModShift
	}
	if win32.KeyDown(win32.VK_CONTROL) {
		m |= input.ModCtrl
	}
	if win32.KeyDown(win32.VK_MENU) {
		m |= input.ModAlt
	}
	if win32.KeyDown(win32.VK_LWIN) || win32.KeyDown(win32.VK_RWIN) {
		m |= input.ModSuper
	}
	return m
}

// keyDown handles a key press before TranslateMessage. It returns true
// when it has sent the key, in which case the message is not translated
// (no WM_CHAR follows).
func (a *App) keyDown(msg *win32.MSG) bool {
	vk := uint32(msg.WParam)
	if vk == win32.VK_PROCESSKEY || a.nv == nil {
		return false // the input method's
	}
	m := mods()
	if vk == win32.VK_F4 && m == input.ModAlt {
		return false // Alt+F4 closes the window
	}
	if k := input.FromVK(vk); k != input.KeyNone {
		if k == input.KeySpace && m&(input.ModCtrl|input.ModAlt|input.ModSuper) == 0 {
			return false // " " comes as WM_CHAR
		}
		a.nv.Input(input.SpecialKey(k, m))
		return true
	}
	if m&(input.ModCtrl|input.ModAlt) == 0 {
		return false
	}
	scan := uint32(msg.LParam>>16) & 0xff
	var st [256]byte
	win32.GetKeyboardState(&st)
	var buf [4]uint16
	// AltGr (right Alt, which Windows reports as Ctrl+Alt) types
	// characters on many layouts: let those through as text.
	if m&input.ModCtrl != 0 && m&input.ModAlt != 0 && win32.KeyDown(win32.VK_RMENU) {
		if n := win32.ToUnicode(vk, scan, &st, buf[:], win32.GetKeyboardLayout()); n == 1 && buf[0] >= 0x20 {
			return false
		}
	}
	// The character the key types with Shift but without Ctrl and Alt.
	for _, k := range []int{win32.VK_CONTROL, win32.VK_LCONTROL, win32.VK_RCONTROL, win32.VK_MENU, win32.VK_LMENU, win32.VK_RMENU} {
		st[k] = 0
	}
	n := win32.ToUnicode(vk, scan, &st, buf[:], win32.GetKeyboardLayout())
	var ch rune
	if n == 1 {
		ch = rune(buf[0])
	} else {
		ch = rune(win32.MapVirtualKey(vk, win32.MAPVK_VK_TO_CHAR) & 0xffff)
		if ch >= 'A' && ch <= 'Z' && m&input.ModShift == 0 {
			ch += 'a' - 'A'
		}
	}
	if ch < 0x20 {
		return false
	}
	if input.IsPaste(ch, m) {
		go func() {
			runtime.LockOSThread() // OpenClipboard and CloseClipboard: same thread
			defer runtime.UnlockOSThread()
			if text, ok := win32.ClipboardText(a.hwnd); ok {
				a.nv.Paste(text)
			}
		}()
		return true
	}
	if s := input.ChordKey(ch, m); s != "" {
		a.nv.Input(s)
		return true
	}
	return false
}

func (a *App) char(c uint16) {
	if utf16.IsSurrogate(rune(c)) {
		if c < 0xDC00 {
			a.highSurrogate = c
			return
		}
		if a.highSurrogate == 0 {
			return
		}
		r := utf16.DecodeRune(rune(a.highSurrogate), rune(c))
		a.highSurrogate = 0
		a.nv.Input(input.Text(string(r)))
		return
	}
	a.highSurrogate = 0
	if c < 0x20 || c == 0x7f {
		return // control characters come as keys
	}
	a.nv.Input(input.Text(string(rune(c))))
}

// Input method

// updateIME turns the input method on or off for the current mode and
// moves its windows to the cursor.
func (a *App) updateIME() {
	want := input.IMEWanted(a.scr.ModeName, a.imeInsertOnly)
	if a.scr.ModeName == "" {
		want = true
	}
	if a.imeForce != 0 {
		want = a.imeForce > 0 // g:ngw_ime
	}
	if want != a.imeEnabled {
		// Leaving text modes: finish any composition, remember whether
		// the input method was on and switch it off, then detach it from
		// the window. Entering them: attach it and restore its state, so
		// it is on again in Insert mode if it was on when Insert mode was
		// left (unless g:ngw_ime_off_on_leave).
		//
		// The command line is the exception: it starts with the input
		// method off, since what is typed there is mostly commands
		// (":w" must not become a composition). Switching it on there
		// is remembered for Insert mode, switching it off is not.
		cmdline := a.imeForce == 0 && strings.HasPrefix(a.scr.ModeName, "cmdline")
		if !want {
			if c := win32.ImmGetContext(a.hwnd); c != 0 {
				win32.ImmNotifyIME(c, win32.NI_COMPOSITIONSTR, win32.CPS_COMPLETE, 0)
				open := win32.ImmGetOpenStatus(c)
				if a.imeFromCmdline {
					a.imeWasOpen = (a.imeWasOpen || open) && !a.imeOffOnLeave
				} else {
					a.imeWasOpen = open && !a.imeOffOnLeave
				}
				win32.ImmSetOpenStatus(c, false)
				win32.ImmReleaseContext(a.hwnd, c)
			}
			win32.ImmEnable(a.hwnd, false)
		} else {
			win32.ImmEnable(a.hwnd, true)
			if c := win32.ImmGetContext(a.hwnd); c != 0 {
				win32.ImmSetOpenStatus(c, a.imeWasOpen && !cmdline)
				win32.ImmReleaseContext(a.hwnd, c)
			}
			a.imeFromCmdline = cmdline
		}
		a.imeEnabled = want
		a.imePos = win32.POINT{X: -1}
	}
	if !want {
		return
	}
	x, y := a.imeCaret()
	if (win32.POINT{X: x, Y: y}) == a.imePos {
		return
	}
	a.imePos = win32.POINT{X: x, Y: y}
	c := win32.ImmGetContext(a.hwnd)
	if c == 0 {
		return
	}
	defer win32.ImmReleaseContext(a.hwnd, c)
	ch := a.fonts.CellH
	cf := win32.COMPOSITIONFORM{Style: win32.CFS_FORCE_POSITION, CurrentPos: win32.POINT{X: x, Y: y}}
	win32.ImmSetCompositionWindow(c, &cf)
	cand := win32.CANDIDATEFORM{Index: 0, Style: win32.CFS_EXCLUDE,
		CurrentPos: win32.POINT{X: x, Y: y + ch},
		Area:       win32.RECT{Left: x, Top: y, Right: x + a.fonts.CellW, Bottom: y + ch}}
	win32.ImmSetCandidateWindow(c, &cand)
}

// imeCaret returns where the candidate list should open: below the start
// of the clause being converted, or at the cursor.
func (a *App) imeCaret() (x, y int32) {
	cw, ch := a.fonts.CellW, a.fonts.CellH
	x, y = int32(a.scr.CursorCol)*cw, int32(a.scr.CursorRow)*ch
	if a.pre == nil || a.pre.text == "" {
		return
	}
	cells := a.r.layoutPreedit(a.pre, a.ambiDouble)
	idx := -1
	for i, c := range cells {
		if c.attr == win32.ATTR_TARGET_CONVERTED || c.attr == win32.ATTR_TARGET_NOTCONVERTED {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = min(a.pre.cursor, len(cells)-1)
	}
	if idx >= 0 && idx < len(cells) {
		x, y = int32(cells[idx].col)*cw, int32(cells[idx].row)*ch
	}
	return
}

func (a *App) imeCharPosition(p *win32.IMECHARPOSITION) {
	cw, ch := a.fonts.CellW, a.fonts.CellH
	x, y := int32(a.scr.CursorCol)*cw, int32(a.scr.CursorRow)*ch
	if a.pre != nil {
		cells := a.r.layoutPreedit(a.pre, a.ambiDouble)
		// CharPos counts UTF-16 units of the composition.
		units := 0
		for _, c := range cells {
			if units >= int(p.CharPos) {
				x, y = int32(c.col)*cw, int32(c.row)*ch
				break
			}
			units += len(utf16.Encode([]rune(c.text)))
		}
	}
	pt := win32.POINT{X: x, Y: y}
	win32.ClientToScreen(a.hwnd, &pt)
	p.Pt = pt
	p.LineHeight = uint32(ch)
	rc := win32.GetClientRect(a.hwnd)
	tl := win32.POINT{X: rc.Left, Y: rc.Top}
	br := win32.POINT{X: rc.Right, Y: rc.Bottom}
	win32.ClientToScreen(a.hwnd, &tl)
	win32.ClientToScreen(a.hwnd, &br)
	p.Document = win32.RECT{Left: tl.X, Top: tl.Y, Right: br.X, Bottom: br.Y}
}

func (a *App) imeComposition(lp uintptr) {
	c := win32.ImmGetContext(a.hwnd)
	if c == 0 {
		return
	}
	defer win32.ImmReleaseContext(a.hwnd, c)
	if lp&win32.GCS_RESULTSTR != 0 {
		if s := utf16Bytes(win32.ImmGetCompositionString(c, win32.GCS_RESULTSTR)); s != "" {
			a.nv.Input(input.Text(s))
		}
		if lp&win32.GCS_COMPSTR == 0 {
			a.setPreedit(nil)
		}
	}
	if lp&win32.GCS_COMPSTR != 0 {
		raw := win32.ImmGetCompositionString(c, win32.GCS_COMPSTR)
		u := bytesToUTF16(raw)
		attrUnits := win32.ImmGetCompositionString(c, win32.GCS_COMPATTR)
		cursorUnits := -1
		if lp&win32.GCS_CURSORPOS != 0 {
			cursorUnits = win32.ImmGetCompositionInt(c, win32.GCS_CURSORPOS)
		}
		p := &preedit{text: string(utf16.Decode(u)), cursor: -1}
		// Attributes and the caret are per UTF-16 unit; convert to runes.
		ri := 0
		for i := 0; i < len(u); i++ {
			if i == cursorUnits {
				p.cursor = ri
			}
			var at byte
			if i < len(attrUnits) {
				at = attrUnits[i]
			}
			p.attrs = append(p.attrs, at)
			if utf16.IsSurrogate(rune(u[i])) && i+1 < len(u) {
				i++
			}
			ri++
		}
		if cursorUnits >= len(u) {
			p.cursor = ri
		}
		if p.text == "" {
			p = nil
		}
		a.setPreedit(p)
	}
	if lp == 0 {
		a.setPreedit(nil)
	}
}

func (a *App) setPreedit(p *preedit) {
	win32.InvalidateRect(a.hwnd, &a.preRect)
	win32.InvalidateRect(a.hwnd, &a.curRect)
	a.pre = p
	a.preRect = win32.RECT{}
	if p != nil {
		cells := a.r.layoutPreedit(p, a.ambiDouble)
		a.preRect = a.r.preeditRect(cells)
		win32.InvalidateRect(a.hwnd, &a.preRect)
	}
	a.updateIME()
}

func bytesToUTF16(b []byte) []uint16 {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return u
}

func utf16Bytes(b []byte) string { return string(utf16.Decode(bytesToUTF16(b))) }

// Mouse

func (a *App) cellAt(lp uintptr) (row, col int) {
	x, y := win32.XLParam(lp), win32.YLParam(lp)
	row = clampInt(y/int(a.fonts.CellH), 0, a.scr.Height-1)
	col = clampInt(x/int(a.fonts.CellW), 0, a.scr.Width-1)
	return
}

func clampInt(v, lo, hi int) int { return max(lo, min(v, hi)) }

func mouseMods(wp uintptr) string {
	var m input.Mod
	if wp&win32.MK_SHIFT != 0 {
		m |= input.ModShift
	}
	if wp&win32.MK_CONTROL != 0 {
		m |= input.ModCtrl
	}
	if win32.KeyDown(win32.VK_MENU) {
		m |= input.ModAlt
	}
	return input.MouseModifier(m)
}

func (a *App) mouseButton(msg uint32, wp, lp uintptr, action string) {
	if !a.scr.MouseEnabled {
		return
	}
	btn := "left"
	switch msg {
	case win32.WM_RBUTTONDOWN, win32.WM_RBUTTONUP:
		btn = "right"
	case win32.WM_MBUTTONDOWN, win32.WM_MBUTTONUP:
		btn = "middle"
	}
	row, col := a.cellAt(lp)
	if action == "press" {
		a.button = btn
		win32.SetCapture(a.hwnd)
	} else {
		if a.button == btn {
			a.button = ""
			win32.ReleaseCapture()
		}
	}
	a.mouseRow, a.mouseCol = row, col
	a.nv.InputMouse(btn, action, mouseMods(wp), row, col)
}

func (a *App) mouseMove(wp, lp uintptr) {
	if a.button == "" || !a.scr.MouseEnabled {
		return
	}
	row, col := a.cellAt(lp)
	if row == a.mouseRow && col == a.mouseCol {
		return
	}
	a.mouseRow, a.mouseCol = row, col
	a.nv.InputMouse(a.button, "drag", mouseMods(wp), row, col)
}

func (a *App) wheel(msg uint32, wp, lp uintptr) {
	if !a.scr.MouseEnabled {
		return
	}
	pt := win32.POINT{X: int32(win32.XLParam(lp)), Y: int32(win32.YLParam(lp))}
	win32.ScreenToClient(a.hwnd, &pt)
	row := clampInt(int(pt.Y)/int(a.fonts.CellH), 0, a.scr.Height-1)
	col := clampInt(int(pt.X)/int(a.fonts.CellW), 0, a.scr.Width-1)
	delta := int(int16(win32.HIWORD(wp)))
	mod := mouseMods(wp)
	if msg == win32.WM_MOUSEHWHEEL {
		a.wheelH += delta
		for ; a.wheelH >= 120; a.wheelH -= 120 {
			a.nv.InputMouse("wheel", "right", mod, row, col)
		}
		for ; a.wheelH <= -120; a.wheelH += 120 {
			a.nv.InputMouse("wheel", "left", mod, row, col)
		}
		return
	}
	a.wheelV += delta
	for ; a.wheelV >= 120; a.wheelV -= 120 {
		a.nv.InputMouse("wheel", "up", mod, row, col)
	}
	for ; a.wheelV <= -120; a.wheelV += 120 {
		a.nv.InputMouse("wheel", "down", mod, row, col)
	}
}
