package policy

import (
	"errors"
	"testing"

	"github.com/AyoubAchour/net-switch/internal/apppath"
	"github.com/AyoubAchour/net-switch/internal/engine/platform"
)

// A flow without an owner (a virtual machine, WSL or a container behind
// Windows' shared connection) follows the pin stored under the reserved
// identity; without that pin it keeps the main line or its combine share.
func TestOwnerlessFlowsFollowTheReservedPin(t *testing.T) {
	t.Run("pinned", func(t *testing.T) {
		c := config()
		c.AppLines[apppath.Unknown] = "B"
		p := compile(t, c)
		for _, port := range []uint16{80, 5000} {
			if d, err := p.Decide(platform.Owner{}, port, false, 1); err != nil || d.Lane != "B" || d.Line != "B" {
				t.Fatalf("port %d: %+v %v", port, d, err)
			}
		}
		if got := p.Predict(apppath.Unknown); got.Line != "B" || got.Distribution != nil || got.Blocked {
			t.Fatalf("prediction %+v", got)
		}
		if got := p.Predict(""); got.Line != "B" {
			t.Fatalf("prediction for no owner %+v", got)
		}
		// An owned app is unaffected by the reserved pin.
		if d, err := p.Decide(platform.Owner{AppID: `c:\apps\other.exe`}, 80, false, 1); err != nil || d.Line != "A" {
			t.Fatalf("owned app: %+v %v", d, err)
		}
	})
	t.Run("unpinned", func(t *testing.T) {
		c := config()
		c.Combine = false
		p := compile(t, c)
		if d, err := p.Decide(platform.Owner{}, 5000, false, 1); err != nil || d.Line != "A" {
			t.Fatalf("main line: %+v %v", d, err)
		}
		c.Combine = true
		p = compile(t, c)
		lines := map[string]bool{}
		for k := uint64(1); k <= 64; k++ {
			d, err := p.Decide(platform.Owner{}, 5000, false, k)
			if err != nil {
				t.Fatal(err)
			}
			lines[d.Line] = true
		}
		if !lines["A"] || !lines["B"] {
			t.Fatalf("combine share not used: %v", lines)
		}
		if got := p.Predict(apppath.Unknown); got.Distribution == nil {
			t.Fatalf("prediction %+v", got)
		}
	})
	t.Run("blocked", func(t *testing.T) {
		c := config()
		c.AppLines[apppath.Unknown] = "B"
		c.Blocked = []string{"B"}
		delete(c.Shares, "B")
		p := compile(t, c)
		if _, err := p.Decide(platform.Owner{}, 5000, false, 1); !errors.Is(err, ErrBlocked) {
			t.Fatal("blocked reserved pin accepted", err)
		}
		if got := p.Predict(apppath.Unknown); !got.Blocked {
			t.Fatalf("prediction %+v", got)
		}
	})
}
