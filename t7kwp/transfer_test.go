package t7kwp

import (
	"bytes"
	"testing"
)

// payloadBytes is how many of a frame's 8 bytes the ECU actually consumes: it
// reads the KWP length byte and stops. Anything past that is padding, so the
// old and new builders are free to disagree there — and they do, since the old
// one filled it by reading past the end of the block.
func payloadBytes(frameIdx, dataLen int) int {
	if frameIdx == 0 {
		return 4 + min(dataLen, 4) // flow byte, 0xA1, length, SID, then payload
	}
	remaining := dataLen - 4 - 6*(frameIdx-1)
	return 2 + min(max(remaining, 0), 6)
}

func TestTransferDataFramesMatchesWriteRange(t *testing.T) {
	bin := make([]byte, 512)
	for i := range bin {
		bin[i] = byte(i*7 + 3)
	}

	for length := 1; length <= 254; length++ {
		got := transferDataFrames(bin[:length])

		// rebuild the reference the way writeRange did, tracking the reused
		// buffer's stale tail
		want := referenceFrames(bin, length)

		if len(got) != len(want) {
			t.Fatalf("length %d: got %d frames, want %d", length, len(got), len(want))
		}
		for i := range got {
			n := payloadBytes(i, length)
			if !bytes.Equal(got[i][:n], want[i][:n]) {
				t.Fatalf("length %d frame %d: got % 02X, want % 02X (first %d bytes)", length, i, got[i], want[i], n)
			}
		}

		// and the frames must carry the data itself, in order
		var payload []byte
		for i, f := range got {
			n := payloadBytes(i, length)
			if i == 0 {
				payload = append(payload, f[4:8]...)
			} else {
				payload = append(payload, f[2:n]...)
			}
		}
		if len(payload) > length {
			payload = payload[:length]
		}
		if !bytes.Equal(payload, bin[:length]) {
			t.Fatalf("length %d: reassembled payload % 02X, want % 02X", length, payload, bin[:length])
		}
	}
}

// referenceFrames is the original writeRange framing with its buffer reuse
// modelled explicitly.
func referenceFrames(bin []byte, length int) [][8]byte {
	rows := (length + 3) / 6
	binPos := 0
	var data [8]byte
	var out [][8]byte
	for i := rows; i >= 0; i-- {
		data[0], data[1] = byte(i), 0xA1
		switch {
		case i == rows:
			data[0] |= 0x40
			data[2] = byte(length + 1)
			data[3] = 0x36
			for fq := 4; fq < 8; fq++ {
				data[fq] = bin[binPos] // the original over-read here for length < 4
				binPos++
			}
		case i == 0:
			left := length - binPos
			for k := range left {
				data[2+k] = bin[binPos]
				binPos++
			}
		default:
			for k := 2; k < 8; k++ {
				data[k] = bin[binPos]
				binPos++
			}
		}
		out = append(out, data)
	}
	return out
}

// TestRequestDownloadFraming pins the two frames the old writeJump sent.
func TestRequestDownloadFraming(t *testing.T) {
	cl := &Client{}
	var addr, length uint32 = 0x07FE00, 0x000200
	msgs := cl.splitRequest2([]byte{
		0x08, REQUEST_DOWNLOAD,
		byte(addr >> 16), byte(addr >> 8), byte(addr), 0x00,
		byte(length >> 16), byte(length >> 8), byte(length),
	})

	want := [][]byte{
		{0x41, 0xA1, 0x08, 0x34, 0x07, 0xFE, 0x00, 0x00},
		{0x00, 0xA1, 0x00, 0x02, 0x00},
	}
	if len(msgs) != len(want) {
		t.Fatalf("got %d frames, want %d", len(msgs), len(want))
	}
	for i, m := range msgs {
		if !bytes.Equal(m.frame.Bytes(), want[i]) {
			t.Errorf("frame %d: got % 02X, want % 02X", i, m.frame.Bytes(), want[i])
		}
	}
	// only the last frame is answered, on the response id
	if msgs[0].rr || !msgs[1].rr {
		t.Errorf("reply expectation: got %v/%v, want false/true", msgs[0].rr, msgs[1].rr)
	}
}
