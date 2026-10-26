package singbox

import (
	"bytes"
	"testing"

	"github.com/AyoubAchour/net-switch/internal/apps"
	"github.com/AyoubAchour/net-switch/internal/control/engine"
	"github.com/AyoubAchour/net-switch/internal/lines"
)

// sing-box has no rule for connections without a process, so the pin for
// traffic no app owns is left out of its rule files: the files are the same
// as without it, and no pattern names a program called "unknown".
func TestReservedPinIsLeftOutOfRuleFiles(t *testing.T) {
	items := []lines.Line{{ID: "aa", Alias: "Ethernet"}, {ID: "bb", Alias: "Wi-Fi"}}
	app := apps.Pin{Path: `C:\Games\game.exe`, LineID: "bb"}
	without, err := RuleFiles(engine.Policy{Lines: items, AppLines: apps.Pins{app}, Blocked: []string{"bb"}})
	if err != nil {
		t.Fatal(err)
	}
	with, err := RuleFiles(engine.Policy{Lines: items, AppLines: apps.Pins{{Path: apps.UnknownApp, LineID: "bb"}, app}, Blocked: []string{"bb"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(with) != len(without) {
		t.Fatal("rule file count changed")
	}
	for i := range with {
		if with[i].Tag != without[i].Tag || !bytes.Equal(with[i].Bytes, without[i].Bytes) {
			t.Fatalf("%s changed:\n%s\n%s", with[i].Tag, with[i].Bytes, without[i].Bytes)
		}
		if bytes.Contains(bytes.ToLower(with[i].Bytes), []byte("unknown")) {
			t.Fatalf("%s names the reserved pin: %s", with[i].Tag, with[i].Bytes)
		}
	}
}
