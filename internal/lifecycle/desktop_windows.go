package lifecycle

import (
	"os"
	"os/exec"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func desktopAutostart(on *bool) (any, error) {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return nil, err
	}
	defer key.Close()
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	value := windows.ComposeCommandLine([]string{exe, "--background"})
	if on != nil {
		if *on {
			err = key.SetStringValue("NetSwitch", value)
		} else {
			err = key.DeleteValue("NetSwitch")
			if err == registry.ErrNotExist {
				err = nil
			}
		}
		if err != nil {
			return nil, err
		}
	}
	current, _, err := key.GetStringValue("NetSwitch")
	if err == registry.ErrNotExist {
		return false, nil
	}
	return current == value, err
}

// edgePath finds the Edge that ships with Windows, from its App Paths entry.
func edgePath() string {
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		key, err := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\msedge.exe`, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		path, _, err := key.GetStringValue("")
		key.Close()
		if err == nil && path != "" {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}
	return ""
}

// openAppWindow shows the window as an app window of its own (Edge app mode:
// no tabs, address bar or bookmarks, and a separate profile so it never mixes
// with the person's browser). It reports false when Edge is missing.
func openAppWindow(url string) bool {
	edge := edgePath()
	if edge == "" {
		return false
	}
	profile := filepath.Join(os.Getenv("LOCALAPPDATA"), "Net Switch", "window")
	cmd := exec.Command(edge, "--app="+url, "--user-data-dir="+profile, "--window-size=1180,800",
		"--no-first-run", "--no-default-browser-check", "--disable-features=Translate")
	if err := cmd.Start(); err != nil {
		return false
	}
	go cmd.Wait()
	return true
}

func ShowPairing(url, code, owner string) error {
	// The app window is opened by this owner-only process, so it carries the
	// one-time code itself and the pairing page submits it; the service still
	// checks that the caller is the enrolled owner.
	if openAppWindow(url + "#code=" + code) {
		return nil
	}
	verb, _ := windows.UTF16PtrFromString("open")
	address, _ := windows.UTF16PtrFromString(url)
	if err := windows.ShellExecute(0, verb, address, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return err
	}
	// This window belongs to the enrolled user's process and session.
	message, _ := windows.UTF16PtrFromString("Net Switch is set up for " + owner + ".\n\nEnter this code in your browser:\n\n" + code + "\n\nIt expires in two minutes.")
	title, _ := windows.UTF16PtrFromString("Connect to Net Switch")
	go func() {
		windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x40)
	}()
	return nil
}
func ShowStatus(text string) error {
	message, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return err
	}
	title, _ := windows.UTF16PtrFromString("Net Switch")
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x40)
	return nil
}
