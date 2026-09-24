//go:build linux

package passthru

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildFakeLib(t *testing.T) string {
	t.Helper()
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler")
	}
	dir := t.TempDir()
	lib := filepath.Join(dir, "libfake.so")
	if out, err := exec.Command(cc, "-shared", "-fPIC", "-o", lib, "testdata/fake.c").CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, out)
	}
	return lib
}

func TestFakeLibrary(t *testing.T) {
	lib := buildFakeLib(t)

	if _, err := New(filepath.Join(t.TempDir(), "missing.so")); err == nil {
		t.Fatal("New on a missing library succeeded")
	}
	pt, err := New(lib)
	if err != nil {
		t.Fatal(err)
	}
	defer pt.Close()

	var dev uint32
	if err := pt.PassThruOpen("", &dev); err != nil || dev != 7 {
		t.Fatalf("Open(\"\"): dev=%d err=%v", dev, err)
	}
	if err := pt.PassThruOpen("dev", &dev); err != nil || dev != 8 {
		t.Fatalf("Open(\"dev\"): dev=%d err=%v", dev, err)
	}
	fw, dll, api, err := pt.PassThruReadVersion(7)
	if err != nil || fw != "1.0" || dll != "fake" || api != "04.04" {
		t.Fatalf("ReadVersion: %q %q %q %v", fw, dll, api, err)
	}

	var ch uint32
	if err := pt.PassThruConnect(7, CAN, CAN_ID_BOTH|CAN_29BIT_ID, 500000, &ch); err != nil || ch != 9 {
		t.Fatalf("Connect: ch=%d err=%v", ch, err)
	}
	if err := pt.PassThruConnect(1, CAN, 0, 0, &ch); !errors.Is(err, ErrInvalidProtocolID) {
		t.Fatalf("Connect bad args: %v", err)
	}

	var msg PassThruMsg
	n, err := pt.PassThruReadMsg(9, &msg, 10)
	if err != nil || n != 1 {
		t.Fatalf("ReadMsg: n=%d err=%v", n, err)
	}
	if msg.ProtocolID != CAN || msg.RxStatus != CAN_29BIT_ID || msg.Timestamp != 1234 ||
		!bytes.Equal(msg.DataBytes(), []byte{0, 0, 2, 0x58, 0xAB, 0xCD}) {
		t.Fatalf("ReadMsg layout: %s", &msg)
	}
	if _, err := pt.PassThruReadMsg(9, &msg, 0); !errors.Is(err, ErrBufferEmpty) {
		t.Fatalf("ReadMsg empty: %v", err)
	}

	tx := PassThruMsg{ProtocolID: CAN, TxFlags: CAN_29BIT_ID, DataSize: 5, ExtraDataIndex: 5}
	binary.BigEndian.PutUint32(tx.Data[:], 0x258)
	tx.Data[4] = 0x42
	num := uint32(1)
	if err := pt.PassThruWriteMsgs(9, &tx, &num, 25); err != nil {
		t.Fatalf("WriteMsgs: %v", err)
	}

	mask := PassThruMsg{ProtocolID: CAN, DataSize: 4, ExtraDataIndex: 4}
	binary.BigEndian.PutUint32(mask.Data[:], 0x7FF)
	pattern := PassThruMsg{ProtocolID: CAN, DataSize: 4, ExtraDataIndex: 4}
	binary.BigEndian.PutUint32(pattern.Data[:], 0x240)
	var filterID uint32
	if err := pt.PassThruStartMsgFilter(9, PASS_FILTER, &mask, &pattern, nil, &filterID); err != nil || filterID != 42 {
		t.Fatalf("StartMsgFilter: id=%d err=%v", filterID, err)
	}

	set := &SCONFIG_LIST{Params: []SCONFIG{{DATA_RATE, 500000}, {J1962_PINS, 0x100}}}
	if err := pt.PassThruIoctl(9, SET_CONFIG, set, nil); err != nil {
		t.Fatalf("SET_CONFIG: %v", err)
	}
	get := &SCONFIG_LIST{Params: []SCONFIG{{Parameter: DATA_RATE}}}
	if err := pt.PassThruIoctl(9, GET_CONFIG, get, nil); err != nil || get.Params[0].Value != 99 {
		t.Fatalf("GET_CONFIG: value=%d err=%v", get.Params[0].Value, err)
	}
	if err := pt.PassThruIoctl(9, CLEAR_RX_BUFFER, nil, nil); err != nil {
		t.Fatalf("CLEAR_RX_BUFFER: %v", err)
	}
	in := &PassThruMsg{DataSize: 3}
	err = pt.PassThruIoctl(9, FAST_INIT, in, &PassThruMsg{})
	if !errors.Is(err, ErrFailed) || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("FAST_INIT should fail with the library's description: %v", err)
	}
	if err := pt.PassThruIoctl(9, READ_VBATT); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("READ_VBATT: %v", err)
	}

	if err := pt.PassThruDisconnect(9); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if err := pt.PassThruClose(7); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := pt.PassThruClose(1); !errors.Is(err, ErrInvalidDeviceID) {
		t.Fatalf("Close bad id: %v", err)
	}
}

func TestFindDLLs(t *testing.T) {
	lib := buildFakeLib(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, libs := FindDLLs(); len(libs) != 0 {
		t.Fatalf("no ~/.passthru: %v", libs)
	}
	dir := filepath.Join(home, ".passthru")
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(lib, filepath.Join(dir, "lib", "libfake.so")); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("good.json", `{"NAME":"Fake","VENDOR":"Acme","CAN":true,"SW_CAN_PS":true,"FUNCTION_LIB":"~/.passthru/lib/libfake.so"}`)
	write("missing.json", `{"NAME":"Gone","VENDOR":"Acme","FUNCTION_LIB":"/nonexistent/lib.so"}`)
	write("broken.json", `{not json`)
	write("notes.txt", `ignored`)

	_, libs := FindDLLs()
	if len(libs) != 1 {
		t.Fatalf("want 1 library, got %v", libs)
	}
	got := libs[0]
	if got.Name != "Acme Fake" || got.FunctionLibrary != filepath.Join(dir, "lib", "libfake.so") ||
		!got.Capabilities.CAN || !got.Capabilities.SWCANPS || got.Capabilities.ISO15765 {
		t.Fatalf("unexpected entry: %+v", got)
	}
}
