package engine

import (
	"testing"

	"github.com/AyoubAchour/net-switch/internal/apppath"
	"github.com/AyoubAchour/net-switch/internal/apps"
	"github.com/AyoubAchour/net-switch/internal/lines"
)

// The pin for traffic no app owns compiles to the reserved identity the
// engine looks up for flows without an owner, under both OS rules.
func TestReservedPinCompilesForBothRules(t *testing.T) {
	for name, rules := range map[string]apppath.Rules{"windows": apppath.Windows, "linux": apppath.Linux} {
		t.Run(name, func(t *testing.T) {
			p := Policy{
				Lines:    []lines.Line{{ID: "a1", Alias: "Ethernet"}, {ID: "b2", Alias: "Wi-Fi"}},
				AppLines: apps.Pins{{Path: apps.UnknownApp, LineID: "b2"}, {Path: "/apps/main", LineID: "a1"}},
			}
			r, err := NativeRevision(Revision{Number: 1, Policy: p}, rules)
			if err != nil {
				t.Fatal(err)
			}
			if got := r.Config.AppLines[apppath.Unknown]; got != "b2" {
				t.Fatalf("reserved pin compiled to %q: %v", got, r.Config.AppLines)
			}
			if len(r.Config.AppLines) != 2 {
				t.Fatalf("unexpected pins %v", r.Config.AppLines)
			}
			if apps.PinNeedsReselection(apps.Pin{Path: apps.UnknownApp}, "") {
				t.Fatal("reserved pin flagged for reselection")
			}
		})
	}
}
