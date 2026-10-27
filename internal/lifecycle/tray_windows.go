package lifecycle

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// The notification-area icon of the desktop process: a click opens the
// window, and its menu turns routing on or off or quits Net Switch. It lives
// as long as the desktop process, with or without the window.

var (
	shell32              = windows.NewLazySystemDLL("shell32.dll")
	procShellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procRegisterMessage  = user32.NewProc("RegisterWindowMessageW")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procAppendMenu       = user32.NewProc("AppendMenuW")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procPostMessage      = user32.NewProc("PostMessageW")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procLoadIcon         = user32.NewProc("LoadIconW")
)

const (
	nimAdd, nimModify, nimDelete    = 0, 1, 2
	nifMessage, nifIcon, nifTip     = 1, 2, 4
	wmNull                          = 0x0000
	wmLButtonUp                     = 0x0202
	wmRButtonUp                     = 0x0205
	wmContextMenu                   = 0x007B
	wmTrayCallback                  = 0x8000 + 1 // WM_APP + 1
	mfString, mfGrayed, mfSep       = 0x0, 0x1, 0x800
	tpmReturnCmd, tpmRightButton    = 0x100, 0x2
	smCxSmIcon, smCySmIcon          = 49, 50
	idiApplication                  = 32512
	menuOpen, menuRouting, menuQuit = 1, 2, 3
)

type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     uintptr
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	Private uint32
}

var tray struct {
	sync.Mutex
	data           notifyIconData
	added          bool
	routing        string
	taskbarCreated uintptr
}

// StartTray shows the icon. Without one (no Explorer shell), the desktop
// keeps working; the window stays reachable from the Start menu.
func StartTray() {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		var module windows.Handle
		_ = windows.GetModuleHandleEx(0, nil, &module)
		className, _ := windows.UTF16PtrFromString("NetSwitchTray")
		class := wndClassEx{LpfnWndProc: windows.NewCallback(trayProc), HInstance: module, LpszClassName: className}
		class.CbSize = uint32(unsafe.Sizeof(class))
		procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
		// A hidden top-level window, not a message-only one, so it also
		// receives TaskbarCreated when Explorer restarts.
		hwnd, _, _ := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, uintptr(module), 0)
		if hwnd == 0 {
			return
		}
		name, _ := windows.UTF16PtrFromString("TaskbarCreated")
		created, _, _ := procRegisterMessage.Call(uintptr(unsafe.Pointer(name)))
		tray.Lock()
		tray.taskbarCreated = created
		tray.data = notifyIconData{HWnd: hwnd, UID: 1, UFlags: nifMessage | nifIcon | nifTip, UCallbackMessage: wmTrayCallback, HIcon: trayIcon()}
		tray.data.CbSize = uint32(unsafe.Sizeof(tray.data))
		setTipLocked()
		addTrayLocked()
		tray.Unlock()
		go promoteTrayIcon()
		var m msg
		for {
			if r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); r == 0 || int32(r) == -1 {
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
}

// promoteTrayIcon shows the icon on the taskbar instead of in Windows 11's
// overflow, where every new icon starts. Explorer records the icon a moment
// after it is added; once the person moves it themselves, their choice
// (an IsPromoted value) is kept.
func promoteTrayIcon() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	for attempt := 0; attempt < 10; attempt++ {
		time.Sleep(time.Second)
		root, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\NotifyIconSettings`, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		names, _ := root.ReadSubKeyNames(-1)
		root.Close()
		for _, name := range names {
			key, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\NotifyIconSettings\`+name, registry.QUERY_VALUE|registry.SET_VALUE)
			if err != nil {
				continue
			}
			path, _, err := key.GetStringValue("ExecutablePath")
			if err == nil && strings.EqualFold(path, exe) {
				if _, _, err := key.GetIntegerValue("IsPromoted"); err == registry.ErrNotExist {
					_ = key.SetDWordValue("IsPromoted", 1)
				}
				key.Close()
				return
			}
			key.Close()
		}
	}
}

// StopTray removes the icon, so none lingers after Net Switch quits.
func StopTray() {
	tray.Lock()
	defer tray.Unlock()
	if tray.added {
		procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&tray.data)))
		tray.added = false
	}
}

// setTrayRouting shows routing's state in the icon's tooltip and menu.
func setTrayRouting(routing string) {
	tray.Lock()
	defer tray.Unlock()
	if tray.routing == routing {
		return
	}
	tray.routing = routing
	setTipLocked()
	if tray.added {
		procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&tray.data)))
	}
}

func addTrayLocked() {
	if r, _, _ := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&tray.data))); r != 0 {
		tray.added = true
	}
}

func setTipLocked() {
	tip := "Net Switch"
	switch tray.routing {
	case "":
	case "on":
		tip = "Net Switch: routing on"
	case "off":
		tip = "Net Switch: routing off"
	case "error":
		tip = "Net Switch: routing stopped"
	default:
		tip = "Net Switch: routing " + tray.routing
	}
	text, _ := windows.UTF16FromString(tip)
	tray.data.SzTip = [128]uint16{}
	copy(tray.data.SzTip[:len(tray.data.SzTip)-1], text)
}

func trayIcon() uintptr {
	cx, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	cy, _, _ := procGetSystemMetrics.Call(smCySmIcon)
	if exe, err := os.Executable(); err == nil {
		if path, err := windows.UTF16PtrFromString(filepath.Join(filepath.Dir(exe), "net-switch.ico")); err == nil {
			if icon, _, _ := procLoadImage.Call(0, uintptr(unsafe.Pointer(path)), imageIcon, cx, cy, lrLoadFromFile); icon != 0 {
				return icon
			}
		}
	}
	icon, _, _ := procLoadIcon.Call(0, idiApplication)
	return icon
}

func trayProc(hwnd, message, wParam, lParam uintptr) uintptr {
	tray.Lock()
	created := tray.taskbarCreated
	tray.Unlock()
	switch {
	case message == wmTrayCallback:
		switch lParam & 0xFFFF {
		case wmLButtonUp:
			requestDesktop(desktopShow, struct{}{})
		case wmRButtonUp, wmContextMenu:
			showTrayMenu(hwnd)
		}
		return 0
	case created != 0 && message == created:
		// Explorer restarted and forgot every icon.
		tray.Lock()
		tray.added = false
		addTrayLocked()
		tray.Unlock()
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, message, wParam, lParam)
	return r
}

func showTrayMenu(hwnd uintptr) {
	tray.Lock()
	routing := tray.routing
	tray.Unlock()
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	appendMenu := func(flags, id uintptr, text string) {
		p, _ := windows.UTF16PtrFromString(text)
		procAppendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(p)))
	}
	appendMenu(mfString, menuOpen, "Open Net Switch")
	switch routing {
	case "on", "starting":
		appendMenu(mfString, menuRouting, "Turn routing off")
	case "":
		appendMenu(mfGrayed, menuRouting, "Turn routing on")
	default:
		appendMenu(mfString, menuRouting, "Turn routing on")
	}
	appendMenu(mfSep, 0, "")
	appendMenu(mfString, menuQuit, "Quit Net Switch")
	var pt struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	// Without the foreground, the menu would not close on an outside click.
	procSetForeground.Call(hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightButton, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	procPostMessage.Call(hwnd, wmNull, 0, 0)
	switch cmd {
	case menuOpen:
		requestDesktop(desktopShow, struct{}{})
	case menuRouting:
		requestDesktop(desktopRouting, routing != "on" && routing != "starting")
	case menuQuit:
		requestDesktop(desktopQuit, struct{}{})
	}
}

// requestDesktop hands a request to the session without blocking the icon.
func requestDesktop[T any](ch chan T, value T) {
	select {
	case ch <- value:
	default:
	}
}
