package gocan

import (
	"bytes"
	"testing"
	"time"
)

// Zero-length payloads are legal frames (e.g. the firmware's 'G' reply to an
// unknown config selector); the parser must not treat size==0 as "no size yet".
func TestReadSerialCommandZeroLengthPayload(t *testing.T) {
	cmd, err := readSerialCommand(bytes.NewReader([]byte{'G', 0x00, 0x00}), time.Second)
	if err != nil {
		t.Fatalf("zero-length frame: %v", err)
	}
	if cmd.Command != 'G' || len(cmd.Data) != 0 {
		t.Fatalf("got command %q with %d data bytes, want 'G' with 0", cmd.Command, len(cmd.Data))
	}
}

func TestReadSerialCommandNormalPayload(t *testing.T) {
	cmd, err := readSerialCommand(bytes.NewReader([]byte{'v', 0x02, 'o', 'k', 'o' + 'k'}), time.Second)
	if err != nil {
		t.Fatalf("normal frame: %v", err)
	}
	if cmd.Command != 'v' || string(cmd.Data) != "ok" {
		t.Fatalf("got command %q data %q, want 'v' %q", cmd.Command, cmd.Data, "ok")
	}
}
