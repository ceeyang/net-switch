package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/AyoubAchour/net-switch/internal/localipc"
	"github.com/AyoubAchour/net-switch/internal/service"
)

// Offline tests replace the transport; production always authenticates the
// real service endpoint.
var dialIPC = localipc.Dial

// desktopShow carries a later launch's or the tray's request to show the
// window; desktopRouting and desktopQuit carry the tray menu's choices.
var (
	desktopShow    = make(chan struct{}, 1)
	desktopRouting = make(chan bool, 1)
	desktopQuit    = make(chan struct{}, 1)
)

// errDesktopQuit ends the desktop process after the tray's Quit.
var errDesktopQuit = errors.New("quit from the tray")

func controlEndpoint(kind string) localipc.Endpoint {
	return localipc.Endpoint{Path: localipc.DefaultPath(kind), Kind: kind, Service: service.Control}
}

func ConnectDesktop(ctx context.Context, status bool) (*RPC, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	kind := "desktop"
	if status {
		kind = "status"
	}
	conn, err := dialIPC(ctx, controlEndpoint(kind))
	if !status && localipc.Unavailable(err) {
		if activateErr := service.Activate(ctx); activateErr != nil {
			return nil, activateErr
		}
		for localipc.Unavailable(err) {
			if pauseErr := localipc.Pause(ctx, 100*time.Millisecond); pauseErr != nil {
				return nil, pauseErr
			}
			conn, err = dialIPC(ctx, controlEndpoint(kind))
		}
	}
	if err != nil {
		return nil, err
	}
	return NewRPC(conn), nil
}

// ConnectStatus opens a connection for status without explicitly activating
// Control. Linux's public fallback retains normal socket activation. Every
// signed-in user may open the public endpoint, so one of them
// can keep it busy (on Windows, idle opens hold its listening instance through
// each bounded identity read). Callers with protected rights (the owner, an
// elevated administrator, SYSTEM or root) therefore read status on the
// protected endpoint with its separate admission and owner access rules.
// Only a denied or missing protected endpoint falls back to
// the public, status-only one; any other failure is reported as is.
func ConnectStatus(ctx context.Context) (*RPC, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, err := dialIPC(ctx, controlEndpoint("desktop"))
	if localipc.Unavailable(err) || errors.Is(err, fs.ErrPermission) {
		conn, err = dialIPC(ctx, controlEndpoint("status"))
	}
	if err != nil {
		return nil, err
	}
	return NewRPC(conn), nil
}

// ConfigureAutostart is the per-user installer hook. It writes only the
// enrolled user's Run key/XDG entry; it does not turn routing on.
func ConfigureAutostart(ctx context.Context, mode string) (any, error) {
	if mode != "on" && mode != "off" && mode != "status" {
		return nil, fmt.Errorf("autostart must be on, off or status")
	}
	if err := service.Activate(ctx); err != nil {
		return nil, err
	}
	rpc, err := ConnectDesktop(ctx, false)
	if err != nil {
		return nil, err
	}
	defer rpc.Close()
	if err := rpc.Call(ctx, map[string]any{"method": "session"}, nil); err != nil {
		return nil, err
	}
	if mode == "status" {
		return DesktopCall(ctx, "autostart", nil)
	}
	return DesktopCall(ctx, "autostart-set", map[string]any{"on": mode == "on"})
}
func Desktop(ctx context.Context, show bool) error {
	// Only the sign-in entry point requests automatic routing. Reopening a
	// window must preserve an explicit Turn off or a latched recovery error.
	first := !show
	backoff := time.Second
	for ctx.Err() == nil {
		connectCtx, stop := context.WithTimeout(ctx, 40*time.Second)
		// The owner uses the protected endpoint first. Public status load
		// cannot prevent a desktop from obtaining its own reserved slot.
		status, err := ConnectDesktop(connectCtx, false)
		if err != nil {
			if err = service.Activate(connectCtx); err == nil {
				status, err = ConnectDesktop(connectCtx, false)
			}
			if err != nil {
				status, err = ConnectDesktop(connectCtx, true)
			}
		}
		if err != nil {
			stop()
			fmt.Println("Net Switch is not ready; retrying:", err)
			if localipc.Pause(ctx, backoff) != nil {
				return nil
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		var state struct {
			Owner   Owner  `json:"owner"`
			Routing string `json:"routing"`
		}
		err = status.Call(connectCtx, map[string]any{"method": "status"}, &state)
		status.Close()
		if err != nil {
			stop()
			if localipc.Pause(ctx, backoff) != nil {
				return nil
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		if state.Owner.User == "" || state.Owner.User != CurrentUser() {
			stop()
			message := "Owner enrolment is pending. Ask an administrator to run netswitchctl enroll."
			if state.Owner.User != "" {
				message = fmt.Sprintf("Net Switch is set up for %s. Routing is %s. This account can view status only.", state.Owner.Name, state.Routing)
			}
			if show {
				return ShowStatus(message)
			}
			fmt.Println(message)
			return nil
		}
		rpc, err := ConnectDesktop(connectCtx, false)
		stop()
		if err != nil {
			if localipc.Pause(ctx, backoff) != nil {
				return nil
			}
			continue
		}
		backoff = time.Second
		registered := false
		// Keep the sign-in intent until the server has acknowledged it.
		if first {
			signCtx, stop := context.WithTimeout(ctx, 40*time.Second)
			err = rpc.Call(signCtx, map[string]any{"method": "signin"}, nil)
			stop()
			var rejected rpcError
			switch {
			case errors.As(err, &rejected):
				// Control answered and refused. Exiting would leave the
				// owner without Net Switch for the whole session, so stay
				// connected, show why, and let them act from the window.
				showRejectedSignIn(fmt.Sprintf("Net Switch did not turn routing on at sign-in: %v", err))
				first = false
			case err != nil:
				rpc.Close()
				if localipc.Pause(ctx, backoff) != nil {
					return nil
				}
				continue
			default:
				first = false
				registered = true
			}
		}
		err = desktopSession(ctx, rpc, show, registered)
		rpc.Close()
		// After a reconnect, pair again only a window that is still open.
		show = desktopWindowOpen()
		if ctx.Err() != nil || errors.Is(err, errDesktopQuit) {
			return nil
		}
		fmt.Println("Net Switch connection lost; reconnecting:", err)
		first = false
		if err := localipc.Pause(ctx, time.Second); err != nil {
			return nil
		}
	}
	return nil
}

// showRejectedSignIn reports a refused sign-in without blocking the desktop.
var showRejectedSignIn = func(message string) {
	fmt.Println(message)
	go func() { _ = ShowStatus(message) }()
}

func desktopSession(ctx context.Context, rpc *RPC, show, registered bool) error {
	var session struct {
		Token string `json:"token"`
		URL   string `json:"url"`
		Owner string `json:"ownerName"`
	}
	if err := rpc.Call(ctx, map[string]any{"method": "session"}, &session); err != nil {
		return err
	}
	if !registered {
		// Reconnect metadata without turning routing back on after the owner
		// explicitly stopped it.
		if err := rpc.Call(ctx, map[string]any{"method": "desktop-connect"}, nil); err != nil {
			return err
		}
	}
	pair := func() error {
		var code string
		if err := rpc.Call(ctx, map[string]any{"method": "pair"}, &code); err != nil {
			return err
		}
		return ShowPairing(session.URL+"pair", code, session.Owner)
	}
	if show {
		if err := pair(); err != nil {
			return err
		}
	}
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	polls := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-desktopShow:
			// Net Switch was opened again: reuse the window, or pair a new one.
			if !focusOpenWindow() {
				if err := pair(); err != nil {
					return err
				}
			}
		case on := <-desktopRouting:
			if err := rpc.Call(ctx, map[string]any{"method": "routing", "on": on}, nil); err != nil {
				go func() {
					_ = ShowStatus(fmt.Sprintf("Net Switch could not turn routing %s: %v", map[bool]string{true: "on", false: "off"}[on], err))
				}()
			}
		case <-desktopQuit:
			// Quit as Node's tray does: routing off, so every app goes back to
			// Windows' normal connection, then close the window and the icon.
			if err := rpc.Call(ctx, map[string]any{"method": "routing", "on": false}, nil); err != nil {
				go func() { _ = ShowStatus(fmt.Sprintf("Net Switch could not turn routing off: %v", err)) }()
				continue
			}
			closeDesktopWindow()
			return errDesktopQuit
		case <-rpc.Done():
			return fmt.Errorf("Control restarted")
		case <-tick.C:
			// Keep the tray's tooltip and menu in step about every two seconds.
			if polls++; polls%8 == 1 {
				var state struct {
					Routing string `json:"routing"`
				}
				if err := rpc.Call(ctx, map[string]any{"method": "status"}, &state); err == nil {
					setTrayRouting(state.Routing)
				}
			}
			var tasks []DesktopWork
			if err := rpc.Call(ctx, map[string]any{"method": "desktop-poll"}, &tasks); err != nil {
				return err
			}
			for _, task := range tasks {
				value, err := DesktopCall(ctx, task.Operation, task.Args)
				message := ""
				if err != nil {
					message = err.Error()
				}
				if err := rpc.Call(ctx, map[string]any{"method": "desktop-result", "key": task.ID, "value": value, "error": message}, nil); err != nil {
					return err
				}
			}
		}
	}
}
