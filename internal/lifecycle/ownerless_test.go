package lifecycle

import "testing"

// Control offers a line for traffic no app owns only once the engine's hello
// advertised the native contract; the sing-box dialect cannot route it.
func TestRemoteRoutesOwnerlessOnlyOnTheNativeEngine(t *testing.T) {
	r := &Remote{}
	if r.RoutesOwnerless() {
		t.Fatal("routes ownerless flows before any hello")
	}
	r.native = true
	if !r.RoutesOwnerless() {
		t.Fatal("native engine does not route ownerless flows")
	}
}
