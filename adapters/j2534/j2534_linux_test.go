//go:build j2534 && linux

package j2534

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	gocan "github.com/roffe/gocan/v2"
)

// Drives the adapter end to end against pkg/passthru/testdata/fake.c: the
// ~/.passthru scan, open on the locked worker thread, filter setup, a
// received frame, a send and the teardown.
func TestFakeLibrary(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".passthru")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(dir, "libfake.so")
	if out, err := exec.Command(cc, "-shared", "-fPIC", "-o", lib, "../../pkg/passthru/testdata/fake.c").CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, out)
	}
	cfg := `{"NAME":"Fake","VENDOR":"Acme","CAN":true,"FUNCTION_LIB":"~/.passthru/libfake.so"}`
	if err := os.WriteFile(filepath.Join(dir, "fake.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	gocan.Rescan()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bus, err := gocan.Open(ctx, "J2534 #0 Acme Fake", gocan.Config{CANRate: 500, CANFilter: []uint32{0x240}})
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()

	select {
	case f := <-bus.Subscribe(ctx, 0x258):
		if !f.Extended || f.Length != 2 || f.Data[0] != 0xAB || f.Data[1] != 0xCD {
			t.Fatalf("unexpected frame: %v", f)
		}
	case <-ctx.Done():
		t.Fatal("no frame delivered")
	}
	if err := bus.Send(ctx, gocan.NewExtendedFrame(0x258, []byte{0x42})); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := bus.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
