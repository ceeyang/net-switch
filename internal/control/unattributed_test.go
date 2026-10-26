package control

import (
	"context"
	"testing"

	"github.com/AyoubAchour/net-switch/internal/apppath"
	"github.com/AyoubAchour/net-switch/internal/apps"
	"github.com/AyoubAchour/net-switch/internal/control/engine"
	js "github.com/AyoubAchour/net-switch/internal/jsvalue"
)

// Traffic without an owning program (virtual machines, WSL, containers) is
// one row the person can place on a line: the row carries the reserved path,
// the pin API stores it, and the revision compiles it to the identity the
// engine uses for flows without an owner. It still can't be reconnected.
func TestUnattributedTrafficCanBePlacedOnALine(t *testing.T) {
	c, e, _, _ := testCore(t, map[string]any{"lines": testLines()})
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	flow := engine.Connection{}
	flow.ID = "1"
	flow.Chains = []string{"line-a1"}
	flow.Start = c.op.Clock.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	e.connections = []engine.Connection{flow}
	if err := c.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	row := stateRows(t, c)[apps.UnknownApp]
	if row == nil || row["path"] != apps.UnknownApp || row["switchable"] != true || row["reconnectable"] != false || row["line"] != nil || row["name"] != unknownAppName {
		t.Fatalf("unattributed row: %+v", row)
	}
	if _, err := c.Action(ctx, "POST", "/api/app-line", map[string]any{"path": row["path"], "line": "b2"}); err != nil {
		t.Fatal(err)
	}
	if pins := c.pinsLocked(); len(pins) != 1 || pins[0].Path != apps.UnknownApp || pins[0].LineID != "b2" {
		t.Fatalf("pin: %+v", pins)
	}
	p := e.Snapshot().Policy
	if len(p.AppLines) != 1 || p.AppLines[0].Path != apps.UnknownApp {
		t.Fatalf("revision did not carry the pin: %+v", p.AppLines)
	}
	r, err := engine.NativeRevision(engine.Revision{Number: 1, Policy: p}, apppath.Windows)
	if err != nil || r.Config.AppLines[apppath.Unknown] != "b2" {
		t.Fatalf("native revision %+v %v", r.Config.AppLines, err)
	}
	row = stateRows(t, c)[apps.UnknownApp]
	if row == nil || js.Text(row["line"]) != "b2" || row["path"] != apps.UnknownApp || row["pinNeedsReselection"] != nil {
		t.Fatalf("pinned row: %+v", row)
	}
	if _, err := c.Action(ctx, "POST", "/api/reconnect", map[string]any{"key": apps.UnknownApp}); err == nil {
		t.Fatal("traffic without a program was reconnected")
	}
	// With no traffic, the pin alone still shows the row with its path.
	e.connections = nil
	c.tracker = apps.NewTracker(0)
	row = stateRows(t, c)[apps.UnknownApp]
	if row == nil || row["path"] != apps.UnknownApp || js.Text(row["line"]) != "b2" || row["switchable"] != true {
		t.Fatalf("pinned row without traffic: %+v", row)
	}
	if _, err := c.Action(ctx, "POST", "/api/app-line", map[string]any{"path": apps.UnknownApp, "line": nil}); err != nil {
		t.Fatal(err)
	}
	if len(c.pinsLocked()) != 0 || len(e.Snapshot().Policy.AppLines) != 0 {
		t.Fatalf("pin survived clearing: %+v", c.pinsLocked())
	}
}
