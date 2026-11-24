//go:build windows

// Package win32 declares the small part of the Win32 API that ngw uses:
// windows and messages (user32), GDI drawing (gdi32), the input method
// (imm32), drag and drop (shell32) and the clipboard. It uses only the
// standard library, so ngw builds without cgo.
package win32

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	imm32    = syscall.NewLazyDLL("imm32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")

	pRegisterClassExW         = user32.NewProc("RegisterClassExW")
	pCreateWindowExW          = user32.NewProc("CreateWindowExW")
	pDefWindowProcW           = user32.NewProc("DefWindowProcW")
	pDestroyWindow            = user32.NewProc("DestroyWindow")
	pShowWindow               = user32.NewProc("ShowWindow")
	pUpdateWindow             = user32.NewProc("UpdateWindow")
	pGetMessageW              = user32.NewProc("GetMessageW")
	pTranslateMessage         = user32.NewProc("TranslateMessage")
	pDispatchMessageW         = user32.NewProc("DispatchMessageW")
	pPostMessageW             = user32.NewProc("PostMessageW")
	pPostQuitMessage          = user32.NewProc("PostQuitMessage")
	pBeginPaint               = user32.NewProc("BeginPaint")
	pEndPaint                 = user32.NewProc("EndPaint")
	pInvalidateRect           = user32.NewProc("InvalidateRect")
	pGetClientRect            = user32.NewProc("GetClientRect")
	pGetWindowRect            = user32.NewProc("GetWindowRect")
	pGetDC                    = user32.NewProc("GetDC")
	pReleaseDC                = user32.NewProc("ReleaseDC")
	pLoadCursorW              = user32.NewProc("LoadCursorW")
	pLoadIconW                = user32.NewProc("LoadIconW")
	pSetCursor                = user32.NewProc("SetCursor")
	pSetWindowTextW           = user32.NewProc("SetWindowTextW")
	pSetWindowPos             = user32.NewProc("SetWindowPos")
	pAdjustWindowRectEx       = user32.NewProc("AdjustWindowRectEx")
	pAdjustWindowRectExForDpi = user32.NewProc("AdjustWindowRectExForDpi")
	pGetDpiForWindow          = user32.NewProc("GetDpiForWindow")
	pSetProcessDpiAwarenessCt = user32.NewProc("SetProcessDpiAwarenessContext")
	pSetProcessDPIAware       = user32.NewProc("SetProcessDPIAware")
	pGetKeyState              = user32.NewProc("GetKeyState")
	pGetKeyboardState         = user32.NewProc("GetKeyboardState")
	pGetKeyboardLayout        = user32.NewProc("GetKeyboardLayout")
	pToUnicodeEx              = user32.NewProc("ToUnicodeEx")
	pMapVirtualKeyW           = user32.NewProc("MapVirtualKeyW")
	pSetCapture               = user32.NewProc("SetCapture")
	pReleaseCapture           = user32.NewProc("ReleaseCapture")
	pSetTimer                 = user32.NewProc("SetTimer")
	pKillTimer                = user32.NewProc("KillTimer")
	pClientToScreen           = user32.NewProc("ClientToScreen")
	pScreenToClient           = user32.NewProc("ScreenToClient")
	pMessageBoxW              = user32.NewProc("MessageBoxW")
	pOpenClipboard            = user32.NewProc("OpenClipboard")
	pCloseClipboard           = user32.NewProc("CloseClipboard")
	pEmptyClipboard           = user32.NewProc("EmptyClipboard")
	pGetClipboardData         = user32.NewProc("GetClipboardData")
	pSetClipboardData         = user32.NewProc("SetClipboardData")
	pGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLongPtrW        = user32.NewProc("SetWindowLongPtrW")
	pMonitorFromWindow        = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	pGetWindowPlacement       = user32.NewProc("GetWindowPlacement")
	pSetWindowPlacement       = user32.NewProc("SetWindowPlacement")
	pFlashWindow              = user32.NewProc("FlashWindow")
	pIsIconic                 = user32.NewProc("IsIconic")
	pSetForegroundWindow      = user32.NewProc("SetForegroundWindow")

	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pCreateFontIndirectW    = gdi32.NewProc("CreateFontIndirectW")
	pGetTextMetricsW        = gdi32.NewProc("GetTextMetricsW")
	pGetTextExtentPoint32W  = gdi32.NewProc("GetTextExtentPoint32W")
	pGetGlyphIndicesW       = gdi32.NewProc("GetGlyphIndicesW")
	pExtTextOutW            = gdi32.NewProc("ExtTextOutW")
	pSetTextColor           = gdi32.NewProc("SetTextColor")
	pSetBkColor             = gdi32.NewProc("SetBkColor")
	pSetBkMode              = gdi32.NewProc("SetBkMode")
	pSetTextAlign           = gdi32.NewProc("SetTextAlign")
	pCreatePen              = gdi32.NewProc("CreatePen")
	pPolyline               = gdi32.NewProc("Polyline")
	pGetStockObject         = gdi32.NewProc("GetStockObject")
	pEnumFontFamiliesExW    = gdi32.NewProc("EnumFontFamiliesExW")
	pGetTextFaceW           = gdi32.NewProc("GetTextFaceW")

	pImmGetContext            = imm32.NewProc("ImmGetContext")
	pImmReleaseContext        = imm32.NewProc("ImmReleaseContext")
	pImmGetCompositionStringW = imm32.NewProc("ImmGetCompositionStringW")
	pImmSetCompositionWindow  = imm32.NewProc("ImmSetCompositionWindow")
	pImmSetCandidateWindow    = imm32.NewProc("ImmSetCandidateWindow")
	pImmSetCompositionFontW   = imm32.NewProc("ImmSetCompositionFontW")
	pImmAssociateContextEx    = imm32.NewProc("ImmAssociateContextEx")
	pImmGetOpenStatus         = imm32.NewProc("ImmGetOpenStatus")
	pImmSetOpenStatus         = imm32.NewProc("ImmSetOpenStatus")
	pImmNotifyIME             = imm32.NewProc("ImmNotifyIME")

	pDragAcceptFiles = shell32.NewProc("DragAcceptFiles")
	pDragQueryFileW  = shell32.NewProc("DragQueryFileW")
	pDragFinish      = shell32.NewProc("DragFinish")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	pGlobalFree       = kernel32.NewProc("GlobalFree")
	pGlobalLock       = kernel32.NewProc("GlobalLock")
	pGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	pGlobalSize       = kernel32.NewProc("GlobalSize")

	pDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

	pChooseFontW = comdlg32.NewProc("ChooseFontW")
)

// Window messages.
const (
	WM_CREATE               = 0x0001
	WM_DESTROY              = 0x0002
	WM_SIZE                 = 0x0005
	WM_SETFOCUS             = 0x0007
	WM_KILLFOCUS            = 0x0008
	WM_PAINT                = 0x000F
	WM_CLOSE                = 0x0010
	WM_ERASEBKGND           = 0x0014
	WM_SETCURSOR            = 0x0020
	WM_GETMINMAXINFO        = 0x0024
	WM_KEYDOWN              = 0x0100
	WM_KEYUP                = 0x0101
	WM_CHAR                 = 0x0102
	WM_DEADCHAR             = 0x0103
	WM_SYSKEYDOWN           = 0x0104
	WM_SYSKEYUP             = 0x0105
	WM_SYSCHAR              = 0x0106
	WM_UNICHAR              = 0x0109
	WM_IME_STARTCOMPOSITION = 0x010D
	WM_IME_ENDCOMPOSITION   = 0x010E
	WM_IME_COMPOSITION      = 0x010F
	WM_TIMER                = 0x0113
	WM_MOUSEMOVE            = 0x0200
	WM_LBUTTONDOWN          = 0x0201
	WM_LBUTTONUP            = 0x0202
	WM_RBUTTONDOWN          = 0x0204
	WM_RBUTTONUP            = 0x0205
	WM_MBUTTONDOWN          = 0x0207
	WM_MBUTTONUP            = 0x0208
	WM_MOUSEWHEEL           = 0x020A
	WM_MOUSEHWHEEL          = 0x020E
	WM_SIZING               = 0x0214
	WM_CAPTURECHANGED       = 0x0215
	WM_ENTERSIZEMOVE        = 0x0231
	WM_EXITSIZEMOVE         = 0x0232
	WM_DROPFILES            = 0x0233
	WM_IME_SETCONTEXT       = 0x0281
	WM_IME_NOTIFY           = 0x0282
	WM_IME_REQUEST          = 0x0288
	WM_DPICHANGED           = 0x02E0
	WM_APP                  = 0x8000
)

// Styles, flags and other constants.
const (
	CS_HREDRAW = 0x0002
	CS_VREDRAW = 0x0001
	CS_DBLCLKS = 0x0008

	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_POPUP            = 0x80000000
	WS_VISIBLE          = 0x10000000
	WS_EX_ACCEPTFILES   = 0x00000010
	CW_USEDEFAULT       = -0x80000000

	SW_HIDE          = 0
	SW_SHOWNORMAL    = 1
	SW_SHOWMAXIMIZED = 3
	SW_SHOW          = 5

	SWP_NOSIZE       = 0x0001
	SWP_NOMOVE       = 0x0002
	SWP_NOZORDER     = 0x0004
	SWP_NOACTIVATE   = 0x0010
	SWP_FRAMECHANGED = 0x0020

	GWL_STYLE = -16

	IDC_ARROW       = 32512
	IDC_IBEAM       = 32513
	IDI_APPLICATION = 32512

	SRCCOPY = 0x00CC0020

	TRANSPARENT = 1
	OPAQUE      = 2

	TA_BASELINE = 24
	TA_LEFT     = 0
	TA_TOP      = 0

	ETO_OPAQUE  = 0x0002
	ETO_CLIPPED = 0x0004

	PS_SOLID   = 0
	NULL_BRUSH = 5

	FW_NORMAL = 400
	FW_BOLD   = 700

	DEFAULT_CHARSET        = 1
	OUT_TT_PRECIS          = 4
	CLIP_DEFAULT_PRECIS    = 0
	ANTIALIASED_QUALITY    = 4
	CLEARTYPE_QUALITY      = 5
	NONANTIALIASED_QUALITY = 3
	DEFAULT_QUALITY        = 0
	FIXED_PITCH            = 1
	FF_MODERN              = 0x30

	GGI_MARK_NONEXISTING_GLYPHS = 1

	VK_SHIFT      = 0x10
	VK_CONTROL    = 0x11
	VK_MENU       = 0x12
	VK_CAPITAL    = 0x14
	VK_LWIN       = 0x5B
	VK_RWIN       = 0x5C
	VK_LCONTROL   = 0xA2
	VK_RCONTROL   = 0xA3
	VK_LMENU      = 0xA4
	VK_RMENU      = 0xA5
	VK_PROCESSKEY = 0xE5
	VK_F4         = 0x73
	VK_F10        = 0x79

	MAPVK_VK_TO_CHAR = 2
	MAPVK_VK_TO_VSC  = 0

	MK_SHIFT   = 0x0004
	MK_CONTROL = 0x0008

	CF_UNICODETEXT = 13
	GMEM_MOVEABLE  = 0x0002

	// IME
	GCS_COMPSTR                 = 0x0008
	GCS_COMPATTR                = 0x0010
	GCS_CURSORPOS               = 0x0080
	GCS_RESULTSTR               = 0x0800
	CFS_POINT                   = 0x0002
	CFS_FORCE_POSITION          = 0x0020
	CFS_CANDIDATEPOS            = 0x0040
	CFS_EXCLUDE                 = 0x0080
	ISC_SHOWUICOMPOSITIONWINDOW = 0x80000000
	IACE_DEFAULT                = 0x0010
	NI_COMPOSITIONSTR           = 0x0015
	CPS_COMPLETE                = 0x0001
	CPS_CANCEL                  = 0x0004
	IMR_COMPOSITIONWINDOW       = 0x0001
	IMR_CANDIDATEWINDOW         = 0x0002
	IMR_QUERYCHARPOSITION       = 0x0006
	ATTR_INPUT                  = 0x00
	ATTR_TARGET_CONVERTED       = 0x01
	ATTR_CONVERTED              = 0x02
	ATTR_TARGET_NOTCONVERTED    = 0x03

	MONITOR_DEFAULTTONEAREST = 2

	DWMWA_USE_IMMERSIVE_DARK_MODE = 20

	CF_SCREENFONTS         = 0x00000001
	CF_INITTOLOGFONTSTRUCT = 0x00000040
	CF_FIXEDPITCHONLY      = 0x00004000
	CF_NOVERTFONTS         = 0x01000000
)

// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2
const dpiPerMonitorV2 = ^uintptr(3) // (HANDLE)-4

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }
type SIZE struct{ CX, CY int32 }

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	Private uint32
}

type WNDCLASSEXW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type PAINTSTRUCT struct {
	Hdc         uintptr
	Erase       int32
	RcPaint     RECT
	Restore     int32
	IncUpdate   int32
	RgbReserved [32]byte
}

type TEXTMETRICW struct {
	Height, Ascent, Descent, InternalLeading, ExternalLeading int32
	AveCharWidth, MaxCharWidth, Weight, Overhang              int32
	DigitizedAspectX, DigitizedAspectY                        int32
	FirstChar, LastChar, DefaultChar, BreakChar               uint16
	Italic, Underlined, StruckOut, PitchAndFamily, CharSet    byte
}

type LOGFONTW struct {
	Height, Width, Escapement, Orientation, Weight       int32
	Italic, Underline, StrikeOut, CharSet                byte
	OutPrecision, ClipPrecision, Quality, PitchAndFamily byte
	FaceName                                             [32]uint16
}

type COMPOSITIONFORM struct {
	Style      uint32
	CurrentPos POINT
	Area       RECT
}

type CANDIDATEFORM struct {
	Index      uint32
	Style      uint32
	CurrentPos POINT
	Area       RECT
}

type IMECHARPOSITION struct {
	Size       uint32
	CharPos    uint32
	Pt         POINT
	LineHeight uint32
	Document   RECT
}

type MINMAXINFO struct {
	Reserved, MaxSize, MaxPosition, MinTrackSize, MaxTrackSize POINT
}

type MONITORINFO struct {
	Size    uint32
	Monitor RECT
	Work    RECT
	Flags   uint32
}

type WINDOWPLACEMENT struct {
	Length    uint32
	Flags     uint32
	ShowCmd   uint32
	MinPos    POINT
	MaxPos    POINT
	NormalPos RECT
}

type CHOOSEFONTW struct {
	Size         uint32
	Owner        uintptr
	DC           uintptr
	LogFont      *LOGFONTW
	PointSize    int32
	Flags        uint32
	Colors       uint32
	CustData     uintptr
	Hook         uintptr
	TemplateName *uint16
	Instance     uintptr
	Style        *uint16
	FontType     uint16
	_            uint16
	SizeMin      int32
	SizeMax      int32
}

func call(p *syscall.LazyProc, a ...uintptr) uintptr {
	r, _, _ := p.Call(a...)
	return r
}

// Available reports whether a procedure exists (newer APIs).
func available(p *syscall.LazyProc) bool { return p.Find() == nil }

func Str(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func LOWORD(v uintptr) uint16 { return uint16(v) }
func HIWORD(v uintptr) uint16 { return uint16(v >> 16) }

// GET_X_LPARAM / GET_Y_LPARAM (signed).
func XLParam(v uintptr) int { return int(int16(uint16(v))) }
func YLParam(v uintptr) int { return int(int16(uint16(v >> 16))) }

func GetModuleHandle() uintptr { return call(pGetModuleHandleW, 0) }

func RegisterClassEx(wc *WNDCLASSEXW) uint16 {
	wc.Size = uint32(unsafe.Sizeof(*wc))
	return uint16(call(pRegisterClassExW, uintptr(unsafe.Pointer(wc))))
}

func CreateWindowEx(exStyle uint32, class, title string, style uint32, x, y, w, h int32, inst uintptr) uintptr {
	return call(pCreateWindowExW, uintptr(exStyle), uintptr(unsafe.Pointer(Str(class))),
		uintptr(unsafe.Pointer(Str(title))), uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, inst, 0)
}

func DefWindowProc(h uintptr, msg uint32, w, l uintptr) uintptr {
	return call(pDefWindowProcW, h, uintptr(msg), w, l)
}
func DestroyWindow(h uintptr)         { call(pDestroyWindow, h) }
func ShowWindow(h uintptr, cmd int32) { call(pShowWindow, h, uintptr(cmd)) }
func UpdateWindow(h uintptr)          { call(pUpdateWindow, h) }
func PostQuitMessage(code int32)      { call(pPostQuitMessage, uintptr(code)) }
func PostMessage(h uintptr, msg uint32, w, l uintptr) bool {
	return call(pPostMessageW, h, uintptr(msg), w, l) != 0
}
func GetMessage(m *MSG) int32 {
	return int32(call(pGetMessageW, uintptr(unsafe.Pointer(m)), 0, 0, 0))
}
func TranslateMessage(m *MSG) { call(pTranslateMessage, uintptr(unsafe.Pointer(m))) }
func DispatchMessage(m *MSG)  { call(pDispatchMessageW, uintptr(unsafe.Pointer(m))) }

func BeginPaint(h uintptr, ps *PAINTSTRUCT) uintptr {
	return call(pBeginPaint, h, uintptr(unsafe.Pointer(ps)))
}
func EndPaint(h uintptr, ps *PAINTSTRUCT) { call(pEndPaint, h, uintptr(unsafe.Pointer(ps))) }
func InvalidateRect(h uintptr, r *RECT) {
	call(pInvalidateRect, h, uintptr(unsafe.Pointer(r)), 0)
}
func GetClientRect(h uintptr) RECT {
	var r RECT
	call(pGetClientRect, h, uintptr(unsafe.Pointer(&r)))
	return r
}
func GetWindowRect(h uintptr) RECT {
	var r RECT
	call(pGetWindowRect, h, uintptr(unsafe.Pointer(&r)))
	return r
}
func GetDC(h uintptr) uintptr           { return call(pGetDC, h) }
func ReleaseDC(h, dc uintptr)           { call(pReleaseDC, h, dc) }
func LoadCursor(id uintptr) uintptr     { return call(pLoadCursorW, 0, id) }
func LoadIcon(inst, id uintptr) uintptr { return call(pLoadIconW, inst, id) }
func SetCursor(c uintptr)               { call(pSetCursor, c) }
func SetWindowText(h uintptr, s string) {
	call(pSetWindowTextW, h, uintptr(unsafe.Pointer(Str(s))))
}
func SetWindowPos(h, after uintptr, x, y, w, ht int32, flags uint32) {
	call(pSetWindowPos, h, after, uintptr(x), uintptr(y), uintptr(w), uintptr(ht), uintptr(flags))
}
func GetWindowLongPtr(h uintptr, idx int32) uintptr {
	return call(pGetWindowLongPtrW, h, uintptr(idx))
}
func SetWindowLongPtr(h uintptr, idx int32, v uintptr) {
	call(pSetWindowLongPtrW, h, uintptr(idx), v)
}

// AdjustWindowRect grows r from a client rectangle to a window rectangle.
func AdjustWindowRect(r *RECT, style, exStyle uint32, dpi uint32) {
	if available(pAdjustWindowRectExForDpi) {
		call(pAdjustWindowRectExForDpi, uintptr(unsafe.Pointer(r)), uintptr(style), 0, uintptr(exStyle), uintptr(dpi))
		return
	}
	call(pAdjustWindowRectEx, uintptr(unsafe.Pointer(r)), uintptr(style), 0, uintptr(exStyle))
}

// SetDPIAware makes the process per-monitor DPI aware (v2 when possible),
// so text is never bitmap-scaled by Windows.
func SetDPIAware() {
	if available(pSetProcessDpiAwarenessCt) {
		if call(pSetProcessDpiAwarenessCt, dpiPerMonitorV2) != 0 {
			return
		}
	}
	if available(pSetProcessDPIAware) {
		call(pSetProcessDPIAware)
	}
}

// GetDpiForWindow returns the window's DPI, or 96 on old systems.
func GetDpiForWindow(h uintptr) uint32 {
	if available(pGetDpiForWindow) {
		if d := uint32(call(pGetDpiForWindow, h)); d != 0 {
			return d
		}
	}
	return 96
}

func GetKeyState(vk int32) int16 { return int16(call(pGetKeyState, uintptr(vk))) }
func KeyDown(vk int32) bool      { return GetKeyState(vk) < 0 }

func GetKeyboardState(s *[256]byte) bool {
	return call(pGetKeyboardState, uintptr(unsafe.Pointer(s))) != 0
}
func GetKeyboardLayout() uintptr { return call(pGetKeyboardLayout, 0) }

// ToUnicode translates a key with the given keyboard state without
// changing the dead key state of the keyboard (flag 4, Windows 10 1607+).
func ToUnicode(vk, scan uint32, state *[256]byte, buf []uint16, hkl uintptr) int32 {
	return int32(call(pToUnicodeEx, uintptr(vk), uintptr(scan), uintptr(unsafe.Pointer(state)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 4, hkl))
}
func MapVirtualKey(code, mapType uint32) uint32 {
	return uint32(call(pMapVirtualKeyW, uintptr(code), uintptr(mapType)))
}

func SetCapture(h uintptr)              { call(pSetCapture, h) }
func ReleaseCapture()                   { call(pReleaseCapture) }
func SetTimer(h, id uintptr, ms uint32) { call(pSetTimer, h, id, uintptr(ms), 0) }
func KillTimer(h, id uintptr)           { call(pKillTimer, h, id) }
func ClientToScreen(h uintptr, p *POINT) {
	call(pClientToScreen, h, uintptr(unsafe.Pointer(p)))
}
func ScreenToClient(h uintptr, p *POINT) {
	call(pScreenToClient, h, uintptr(unsafe.Pointer(p)))
}
func MessageBox(h uintptr, text, title string, flags uint32) {
	call(pMessageBoxW, h, uintptr(unsafe.Pointer(Str(text))), uintptr(unsafe.Pointer(Str(title))), uintptr(flags))
}
func FlashWindow(h uintptr)         { call(pFlashWindow, h, 1) }
func SetForegroundWindow(h uintptr) { call(pSetForegroundWindow, h) }
func IsIconic(h uintptr) bool       { return call(pIsIconic, h) != 0 }

func MonitorWorkArea(h uintptr) (mon, work RECT) {
	m := call(pMonitorFromWindow, h, MONITOR_DEFAULTTONEAREST)
	var mi MONITORINFO
	mi.Size = uint32(unsafe.Sizeof(mi))
	call(pGetMonitorInfoW, m, uintptr(unsafe.Pointer(&mi)))
	return mi.Monitor, mi.Work
}
func GetWindowPlacement(h uintptr, p *WINDOWPLACEMENT) {
	p.Length = uint32(unsafe.Sizeof(*p))
	call(pGetWindowPlacement, h, uintptr(unsafe.Pointer(p)))
}
func SetWindowPlacement(h uintptr, p *WINDOWPLACEMENT) {
	p.Length = uint32(unsafe.Sizeof(*p))
	call(pSetWindowPlacement, h, uintptr(unsafe.Pointer(p)))
}

// SetDarkTitleBar asks DWM for a dark (or light) caption, Windows 10 20H1+.
func SetDarkTitleBar(h uintptr, dark bool) {
	if !available(pDwmSetWindowAttribute) {
		return
	}
	var v int32
	if dark {
		v = 1
	}
	call(pDwmSetWindowAttribute, h, DWMWA_USE_IMMERSIVE_DARK_MODE, uintptr(unsafe.Pointer(&v)), 4)
}

// GDI

func CreateCompatibleDC(dc uintptr) uintptr { return call(pCreateCompatibleDC, dc) }
func CreateCompatibleBitmap(dc uintptr, w, h int32) uintptr {
	return call(pCreateCompatibleBitmap, dc, uintptr(w), uintptr(h))
}
func DeleteDC(dc uintptr)                { call(pDeleteDC, dc) }
func DeleteObject(o uintptr)             { call(pDeleteObject, o) }
func SelectObject(dc, o uintptr) uintptr { return call(pSelectObject, dc, o) }
func GetStockObject(i int32) uintptr     { return call(pGetStockObject, uintptr(i)) }
func BitBlt(dst uintptr, x, y, w, h int32, src uintptr, sx, sy int32) {
	call(pBitBlt, dst, uintptr(x), uintptr(y), uintptr(w), uintptr(h), src, uintptr(sx), uintptr(sy), SRCCOPY)
}
func CreateFontIndirect(lf *LOGFONTW) uintptr {
	return call(pCreateFontIndirectW, uintptr(unsafe.Pointer(lf)))
}
func GetTextMetrics(dc uintptr, tm *TEXTMETRICW) {
	call(pGetTextMetricsW, dc, uintptr(unsafe.Pointer(tm)))
}
func GetTextExtent(dc uintptr, s []uint16) SIZE {
	var sz SIZE
	if len(s) > 0 {
		call(pGetTextExtentPoint32W, dc, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)), uintptr(unsafe.Pointer(&sz)))
	}
	return sz
}

// HasGlyphs reports whether the font selected in dc has a glyph for every
// UTF-16 unit of s (BMP characters only).
func HasGlyphs(dc uintptr, s []uint16) bool {
	if len(s) == 0 {
		return true
	}
	var buf [8]uint16
	idx := buf[:]
	if len(s) > len(buf) {
		idx = make([]uint16, len(s))
	}
	r := call(pGetGlyphIndicesW, dc, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)),
		uintptr(unsafe.Pointer(&idx[0])), GGI_MARK_NONEXISTING_GLYPHS)
	if r == 0xFFFFFFFF || r == 0 {
		return false
	}
	for _, g := range idx[:len(s)] {
		if g == 0xFFFF {
			return false
		}
	}
	return true
}

func GetTextFace(dc uintptr) string {
	var buf [64]uint16
	call(pGetTextFaceW, dc, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf[:])
}

// ExtTextOut draws s at (x, y). dx gives the advance of each UTF-16 unit;
// clip, if not nil, clips (ETO_CLIPPED) and/or fills (ETO_OPAQUE) it.
func ExtTextOut(dc uintptr, x, y int32, opts uint32, clip *RECT, s []uint16, dx []int32) {
	var sp, dp uintptr
	if len(s) > 0 {
		sp = uintptr(unsafe.Pointer(&s[0]))
	}
	if len(dx) > 0 {
		dp = uintptr(unsafe.Pointer(&dx[0]))
	}
	call(pExtTextOutW, dc, uintptr(x), uintptr(y), uintptr(opts), uintptr(unsafe.Pointer(clip)), sp, uintptr(len(s)), dp)
}

// FillRect fills r with color using ExtTextOut(ETO_OPAQUE), the fastest
// solid fill GDI has; it changes the DC's background color.
func FillRect(dc uintptr, r *RECT, color uint32) {
	call(pSetBkColor, dc, uintptr(color))
	call(pExtTextOutW, dc, 0, 0, ETO_OPAQUE, uintptr(unsafe.Pointer(r)), 0, 0, 0)
}

func SetTextColor(dc uintptr, c uint32) { call(pSetTextColor, dc, uintptr(c)) }
func SetBkColor(dc uintptr, c uint32)   { call(pSetBkColor, dc, uintptr(c)) }
func SetBkMode(dc uintptr, m int32)     { call(pSetBkMode, dc, uintptr(m)) }
func SetTextAlign(dc uintptr, a uint32) { call(pSetTextAlign, dc, uintptr(a)) }
func CreatePen(style, width int32, color uint32) uintptr {
	return call(pCreatePen, uintptr(style), uintptr(width), uintptr(color))
}
func Polyline(dc uintptr, pts []POINT) {
	if len(pts) > 1 {
		call(pPolyline, dc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	}
}

var (
	fontExistsCB    = syscall.NewCallback(fontExistsProc)
	fontExistsFound bool
)

func fontExistsProc(lf, tm, typ, lparam uintptr) uintptr {
	fontExistsFound = true
	return 0
}

// FontExists reports whether a font family named face is installed. It
// matches both English and localized family names. UI thread only.
func FontExists(dc uintptr, face string) bool {
	var lf LOGFONTW
	lf.CharSet = DEFAULT_CHARSET
	u, _ := syscall.UTF16FromString(face)
	if len(u) > 32 {
		return false
	}
	copy(lf.FaceName[:], u)
	fontExistsFound = false
	call(pEnumFontFamiliesExW, dc, uintptr(unsafe.Pointer(&lf)), fontExistsCB, 0, 0)
	return fontExistsFound
}

// ChooseFont shows the font dialog for fixed-pitch fonts. It returns false
// when the user cancels.
func ChooseFont(owner uintptr, lf *LOGFONTW) (pointTenths int32, ok bool) {
	var cf CHOOSEFONTW
	cf.Size = uint32(unsafe.Sizeof(cf))
	cf.Owner = owner
	cf.LogFont = lf
	cf.Flags = CF_SCREENFONTS | CF_INITTOLOGFONTSTRUCT | CF_FIXEDPITCHONLY | CF_NOVERTFONTS
	if call(pChooseFontW, uintptr(unsafe.Pointer(&cf))) == 0 {
		return 0, false
	}
	return cf.PointSize, true
}

// IME

func ImmGetContext(h uintptr) uintptr { return call(pImmGetContext, h) }
func ImmReleaseContext(h, c uintptr)  { call(pImmReleaseContext, h, c) }

// ImmGetCompositionString returns the raw bytes of one composition part.
func ImmGetCompositionString(c uintptr, index uint32) []byte {
	n := int32(call(pImmGetCompositionStringW, c, uintptr(index), 0, 0))
	if n <= 0 {
		return nil
	}
	buf := make([]byte, n)
	call(pImmGetCompositionStringW, c, uintptr(index), uintptr(unsafe.Pointer(&buf[0])), uintptr(n))
	return buf
}

// ImmGetCompositionInt returns GCS_CURSORPOS and similar values.
func ImmGetCompositionInt(c uintptr, index uint32) int {
	return int(int32(call(pImmGetCompositionStringW, c, uintptr(index), 0, 0)))
}

func ImmSetCompositionWindow(c uintptr, f *COMPOSITIONFORM) {
	call(pImmSetCompositionWindow, c, uintptr(unsafe.Pointer(f)))
}
func ImmSetCandidateWindow(c uintptr, f *CANDIDATEFORM) {
	call(pImmSetCandidateWindow, c, uintptr(unsafe.Pointer(f)))
}
func ImmSetCompositionFont(c uintptr, lf *LOGFONTW) {
	call(pImmSetCompositionFontW, c, uintptr(unsafe.Pointer(lf)))
}

// ImmEnable associates the default input context with the window (on) or
// none at all (off): with no context, keys reach the window directly even
// if the input method is switched on.
func ImmEnable(h uintptr, on bool) {
	if on {
		call(pImmAssociateContextEx, h, 0, IACE_DEFAULT)
	} else {
		call(pImmAssociateContextEx, h, 0, 0)
	}
}
func ImmGetOpenStatus(c uintptr) bool { return call(pImmGetOpenStatus, c) != 0 }
func ImmSetOpenStatus(c uintptr, open bool) {
	var v uintptr
	if open {
		v = 1
	}
	call(pImmSetOpenStatus, c, v)
}
func ImmNotifyIME(c uintptr, action, index, value uint32) {
	call(pImmNotifyIME, c, uintptr(action), uintptr(index), uintptr(value))
}

// Drag and drop

func DragAcceptFiles(h uintptr, accept bool) {
	var v uintptr
	if accept {
		v = 1
	}
	call(pDragAcceptFiles, h, v)
}

// DroppedFiles returns the paths of a WM_DROPFILES message and releases it.
func DroppedFiles(hdrop uintptr) []string {
	defer call(pDragFinish, hdrop)
	n := int(call(pDragQueryFileW, hdrop, 0xFFFFFFFF, 0, 0))
	var out []string
	for i := 0; i < n; i++ {
		l := int(call(pDragQueryFileW, hdrop, uintptr(i), 0, 0))
		buf := make([]uint16, l+1)
		call(pDragQueryFileW, hdrop, uintptr(i), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		out = append(out, syscall.UTF16ToString(buf))
	}
	return out
}

// Clipboard

func openClipboard(h uintptr) bool {
	for i := 0; i < 20; i++ {
		if call(pOpenClipboard, h) != 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// ClipboardText returns the clipboard's text (CF_UNICODETEXT).
func ClipboardText(h uintptr) (string, bool) {
	if !openClipboard(h) {
		return "", false
	}
	defer call(pCloseClipboard)
	mem := call(pGetClipboardData, CF_UNICODETEXT)
	if mem == 0 {
		return "", true
	}
	p := call(pGlobalLock, mem)
	if p == 0 {
		return "", false
	}
	defer call(pGlobalUnlock, mem)
	n := int(call(pGlobalSize, mem)) / 2
	u := unsafe.Slice((*uint16)(unsafe.Pointer(p)), n)
	for i, c := range u {
		if c == 0 {
			u = u[:i]
			break
		}
	}
	return syscall.UTF16ToString(u), true
}

// SetClipboardText replaces the clipboard's contents with text.
func SetClipboardText(h uintptr, text string) bool {
	u, err := syscall.UTF16FromString(text)
	if err != nil {
		// text contains NUL: keep what comes before it
		u = append(utf16Prefix(text), 0)
	}
	if !openClipboard(h) {
		return false
	}
	defer call(pCloseClipboard)
	call(pEmptyClipboard)
	mem := call(pGlobalAlloc, GMEM_MOVEABLE, uintptr(len(u)*2))
	if mem == 0 {
		return false
	}
	p := call(pGlobalLock, mem)
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(u)), u)
	call(pGlobalUnlock, mem)
	if call(pSetClipboardData, CF_UNICODETEXT, mem) == 0 {
		call(pGlobalFree, mem)
		return false
	}
	return true
}

func utf16Prefix(s string) []uint16 {
	for i, r := range s {
		if r == 0 {
			u, _ := syscall.UTF16FromString(s[:i])
			return u[:len(u)-1]
		}
	}
	return nil
}
