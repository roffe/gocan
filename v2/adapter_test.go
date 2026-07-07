package gocan

import (
	"slices"
	"testing"
)

func TestRescan(t *testing.T) {
	mk := func(name string) AdapterInfo {
		return AdapterInfo{Name: name, New: func(Config) (Adapter, error) { return &Loopback{}, nil }}
	}
	if err := Register(mk("rescan-static")); err != nil {
		t.Fatal(err)
	}
	devices := []string{"rescan-dev-1", "rescan-dev-2"}
	RegisterScanner(func() []AdapterInfo {
		var out []AdapterInfo
		for _, d := range devices {
			out = append(out, mk(d))
		}
		return out
	})

	// scanner ran once at registration
	names := AdapterNames()
	for _, want := range []string{"rescan-static", "rescan-dev-1", "rescan-dev-2"} {
		if !slices.Contains(names, want) {
			t.Fatalf("missing %s after RegisterScanner: %v", want, names)
		}
	}

	// dev-1 unplugged, dev-3 plugged in
	devices = []string{"rescan-dev-2", "rescan-dev-3"}
	Rescan()

	names = AdapterNames()
	for _, want := range []string{"rescan-static", "rescan-dev-2", "rescan-dev-3"} {
		if !slices.Contains(names, want) {
			t.Fatalf("missing %s after Rescan: %v", want, names)
		}
	}
	if slices.Contains(names, "rescan-dev-1") {
		t.Fatalf("rescan-dev-1 should be gone after Rescan: %v", names)
	}
}
