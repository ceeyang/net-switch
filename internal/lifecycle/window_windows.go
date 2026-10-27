package lifecycle

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

// The window is a native Win32 window that hosts the page in WebView2, the
// engine Windows ships, so Net Switch is its own program on the taskbar
// instead of a browser tab or an Edge app window.

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procShowWindow       = user32.NewProc("ShowWindow")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procIsIconic         = user32.NewProc("IsIconic")
	procSendMessage      = user32.NewProc("SendMessageW")
	procLoadImage        = user32.NewProc("LoadImageW")
	procAllowSetForegrnd = user32.NewProc("AllowSetForegroundWindow")
)

const (
	swRestore       = 9
	wmSetIcon       = 0x0080
	imageIcon       = 1
	lrDefaultSize   = 0x0040
	lrLoadFromFile  = 0x0010
	asfwAny         = ^uintptr(0)
	windowIconID    = 1
	instanceMutex   = `Local\NetSwitchDesktop`
	instanceShowEvt = `Local\NetSwitchDesktopShow`
)

var (
	windowMu      sync.Mutex
	currentWindow webview2.WebView
)

// openNativeWindow shows url in the app's own window, reusing the open one.
// It reports false when WebView2 cannot start, so the caller can fall back.
func openNativeWindow(address string) bool {
	windowMu.Lock()
	if w := currentWindow; w != nil {
		windowMu.Unlock()
		w.Dispatch(func() {
			w.Navigate(address)
			focusWindow(uintptr(w.Window()))
		})
		return true
	}
	windowMu.Unlock()
	ready := make(chan bool, 1)
	go func() {
		// Every call on the window must come from the thread that made it.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			DataPath:  filepath.Join(os.Getenv("LOCALAPPDATA"), "Net Switch", "webview"),
			AutoFocus: true,
			WindowOptions: webview2.WindowOptions{
				Title:  "Net Switch",
				Width:  1180,
				Height: 800,
				IconId: windowIconID,
				Center: true,
			},
		})
		if w == nil {
			ready <- false
			return
		}
		setWindowIcon(uintptr(w.Window()))
		w.SetSize(720, 520, webview2.HintMin)
		_ = w.Bind("netswitchOpenExternal", openExternal)
		// Download and release-note links leave the app for the default browser.
		w.Init(`document.addEventListener('click', e => {
  const a = e.target.closest && e.target.closest('a[target="_blank"]');
  if (a && /^https:/.test(a.href)) { e.preventDefault(); window.netswitchOpenExternal(a.href); }
}, true);`)
		w.Navigate(address)
		windowMu.Lock()
		currentWindow = w
		windowMu.Unlock()
		ready <- true
		focusWindow(uintptr(w.Window()))
		w.Run()
		windowMu.Lock()
		currentWindow = nil
		windowMu.Unlock()
	}()
	return <-ready
}

// closeDesktopWindow closes the window, if one is open.
func closeDesktopWindow() {
	windowMu.Lock()
	w := currentWindow
	windowMu.Unlock()
	if w != nil {
		w.Destroy()
	}
}

// desktopWindowOpen reports whether the person still has the window open.
func desktopWindowOpen() bool {
	windowMu.Lock()
	defer windowMu.Unlock()
	return currentWindow != nil
}

// focusOpenWindow brings the open window forward; false when none is open.
func focusOpenWindow() bool {
	windowMu.Lock()
	w := currentWindow
	windowMu.Unlock()
	if w == nil {
		return false
	}
	w.Dispatch(func() { focusWindow(uintptr(w.Window())) })
	return true
}

func focusWindow(hwnd uintptr) {
	if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
		procShowWindow.Call(hwnd, swRestore)
	}
	procSetForeground.Call(hwnd)
}

// setWindowIcon uses the icon compiled into the program, else the one the
// installer puts next to it.
func setWindowIcon(hwnd uintptr) {
	var module windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &module)
	icon, _, _ := procLoadImage.Call(uintptr(module), windowIconID, imageIcon, 0, 0, lrDefaultSize)
	if icon == 0 {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		path, err := windows.UTF16PtrFromString(filepath.Join(filepath.Dir(exe), "net-switch.ico"))
		if err != nil {
			return
		}
		icon, _, _ = procLoadImage.Call(0, uintptr(unsafe.Pointer(path)), imageIcon, 0, 0, lrDefaultSize|lrLoadFromFile)
	}
	if icon != 0 {
		procSendMessage.Call(hwnd, wmSetIcon, 0, icon)
		procSendMessage.Call(hwnd, wmSetIcon, 1, icon)
	}
}

func openExternal(link string) error {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "https" {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("open")
	address, err := windows.UTF16PtrFromString(parsed.String())
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, address, nil, nil, windows.SW_SHOWNORMAL)
}

// ClaimDesktop makes this the only desktop process of the session. A later
// launch asks the running one to show its window and reports false.
func ClaimDesktop(show bool) bool {
	name, _ := windows.UTF16PtrFromString(instanceMutex)
	mutex, err := windows.CreateMutex(nil, false, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		if mutex != 0 {
			windows.CloseHandle(mutex)
		}
		if show {
			event, _ := windows.UTF16PtrFromString(instanceShowEvt)
			if handle, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, event); err == nil {
				// Let the running process take the foreground this launch was given.
				procAllowSetForegrnd.Call(asfwAny)
				windows.SetEvent(handle)
				windows.CloseHandle(handle)
			}
		}
		return false
	}
	if err != nil {
		return true
	}
	// The mutex handle lives as long as the process.
	event, _ := windows.UTF16PtrFromString(instanceShowEvt)
	handle, err := windows.CreateEvent(nil, 0, 0, event)
	if err != nil {
		return true
	}
	go func() {
		for {
			if result, _ := windows.WaitForSingleObject(handle, windows.INFINITE); result != windows.WAIT_OBJECT_0 {
				return
			}
			select {
			case desktopShow <- struct{}{}:
			default:
			}
		}
	}()
	return true
}
