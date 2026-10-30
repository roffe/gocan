package t7kwp

import (
	"context"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gocantest"
)

// fakeT7 answers like a T7 in a diagnostic session, with the download half
// of EOL programming. Requests are reassembled from 0x240 rows that count
// down to 0, a row with 0x80 set is confirmed on 0x270, and replies go out on
// 0x258 in ack mode: one row at a time, the next only after a 0x266 ack that
// names the row just sent. A wrong ack abandons the reply, and the first row
// of a new request drops whatever reply is still going out.
type fakeT7 struct {
	closed bool // no session: only startCommunication on 0x220 is answered

	pair    SeedKey // the ECU's seed/key algorithm
	seed    uint16  // level 05 seed
	l3Seed  uint16  // level 03 seed
	key     uint16  // key the ECU expects for the last seed it gave out
	access  byte
	badKeys int
	locked  bool // after the third wrong key every securityAccess is refused for a while

	ids map[byte][]byte // readECUIdentification fields
	mem []byte          // read by uploads and readMemoryByAddress, written by writeDataByAddress

	mode    byte // transfer in progress: REQUEST_UPLOAD, REQUEST_DOWNLOAD or 0
	upAddr  int
	upLeft  int
	upBlock int // data bytes per transferData reply
	upCount byte

	flash         []byte // written by requestDownload + transferData
	dlAddr, dlEnd int

	busy   map[byte]int                 // busyRepeatRequest replies to give a SID before answering it
	answer map[byte]func([]byte) []byte // replaces the default answer to a SID; nil means silence

	requests [][]byte // every complete request, as reassembled from its rows

	req     []byte
	reqNext int
	rows    []gocan.Frame
	lastRow byte
	waiting bool
}

func (e *fakeT7) node() *gocantest.Node {
	if e.upBlock == 0 {
		e.upBlock = 250
	}
	if e.ids == nil {
		e.ids = map[byte][]byte{}
	}
	return &gocantest.Node{Reply: e.reply}
}

func newT7(t *testing.T, e *fakeT7) (*Client, *gocantest.Node) {
	t.Helper()
	node := e.node()
	return newClient(t, node), node
}

func (e *fakeT7) reply(f gocan.Frame) []gocan.Frame {
	switch f.ID {
	case INIT_MSG_ID:
		if !e.closed || f.Data[1] != START_COM_REQ || f.Data[2] != 0x00 || f.Data[3] != 0x11 {
			return nil
		}
		e.closed = false
		return []gocan.Frame{gocan.NewFrame(INIT_RESP_ID, []byte{0x40, 0xBF, 0x21, 0xC1, 0x00, 0x11, 0x02, 0x58})}
	case REQ_MSG_ID:
		return e.row(f)
	case RESP_CHUNK_CONF_ID:
		return e.acked(f.Data[3])
	}
	return nil
}

func (e *fakeT7) row(f gocan.Frame) []gocan.Frame {
	seg := int(f.Data[0] & 0x3F)
	switch {
	case f.Data[0]&0x40 != 0:
		e.rows, e.waiting = nil, false
		e.req = append([]byte(nil), f.Data[2:]...)
	case e.req != nil && seg == e.reqNext:
		e.req = append(e.req, f.Data[2:]...)
	default:
		e.req = nil
		return nil
	}
	var out []gocan.Frame
	if f.Data[0]&0x80 != 0 {
		out = append(out, gocan.NewFrame(REQ_CHUNK_CONF_ID, []byte{0x40, 0xBF, 0x21, byte(seg) | 0x80, 0, 0, 0, 0}))
	}
	if seg != 0 {
		e.reqNext = seg - 1
		return out
	}
	msg := e.req
	e.req = nil
	e.requests = append(e.requests, msg)
	if e.closed {
		return out
	}
	if p := e.handle(msg); p != nil {
		out = append(out, e.respond(p))
	}
	return out
}

// respond frames a reply [len, payload...] 6 bytes per row and sends the first.
func (e *fakeT7) respond(p []byte) gocan.Frame {
	msg := append([]byte{byte(len(p))}, p...)
	n := (len(msg) + 5) / 6
	e.rows = nil
	for i := range n {
		var buf [8]byte
		buf[0], buf[1] = 0x80|byte(n-1-i), 0xBF
		if i == 0 {
			buf[0] |= 0x40
		}
		copy(buf[2:], msg[i*6:min(i*6+6, len(msg))])
		e.rows = append(e.rows, gocan.NewFrame(0x258, buf[:]))
	}
	return e.nextRow()
}

func (e *fakeT7) nextRow() gocan.Frame {
	f := e.rows[0]
	e.rows = e.rows[1:]
	e.lastRow, e.waiting = f.Data[0]&0x3F, true
	return f
}

func (e *fakeT7) acked(b byte) []gocan.Frame {
	if !e.waiting {
		return nil
	}
	if b&0x80 == 0 || b&0x3F != e.lastRow {
		e.rows, e.waiting = nil, false
		return nil
	}
	if len(e.rows) == 0 {
		e.waiting = false
		return nil
	}
	return []gocan.Frame{e.nextRow()}
}

func (e *fakeT7) handle(msg []byte) []byte {
	m := make([]byte, max(len(msg), 16))
	copy(m, msg)
	sid := m[1]
	nrc := func(code byte) []byte { return []byte{0x7F, sid, code} }
	if e.busy[sid] > 0 {
		e.busy[sid]--
		return nrc(BUSY_REPEAT_REQUEST)
	}
	if fn, ok := e.answer[sid]; ok {
		return fn(m)
	}
	u24 := func(b []byte) int { return int(b[0])<<16 | int(b[1])<<8 | int(b[2]) }
	switch sid {
	case STOP_COM_REQ:
		e.closed, e.access, e.mode = true, NO_PRIORITY, 0
		return []byte{STOP_COM_REQ | 0x40}
	case TESTER_PRESENT:
		return []byte{TESTER_PRESENT | 0x40}
	case ECU_RESET:
		if m[2] != 0x01 {
			return nrc(SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT)
		}
		e.closed, e.access, e.mode = true, NO_PRIORITY, 0
		return []byte{ECU_RESET | 0x40, 0x81}
	case SECURITY_ACCESS:
		return e.securityAccess(m)
	case READ_ECU_IDENTIFICATION:
		v, ok := e.ids[m[2]]
		if !ok {
			return nrc(SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT)
		}
		return append([]byte{READ_ECU_IDENTIFICATION | 0x40, m[2]}, v...)
	case CLEAR_DTC:
		return []byte{CLEAR_DTC | 0x40, m[2], m[3]}
	case READ_MEMORY_BY_ADDRESS:
		if e.access != DEVELOPMENT_PRIORITY {
			return nrc(SECURITY_ACCESS_DENIED_OR_REQUESTED)
		}
		a, n := u24(m[2:]), int(m[5])
		if a+n > len(e.mem) {
			return nrc(REQUEST_OUT_OF_RANGE)
		}
		return append([]byte{READ_MEMORY_BY_ADDRESS | 0x40, m[2], m[3], m[4]}, e.mem[a:a+n]...)
	case WRITE_DATA_BY_ADDRESS:
		if e.access != DEVELOPMENT_PRIORITY {
			return nrc(SECURITY_ACCESS_DENIED_OR_REQUESTED)
		}
		a, n := u24(m[2:]), int(m[5])
		if a+n > len(e.mem) || 6+n > len(m) {
			return nrc(CANNOT_DOWNLOAD_TO_SPECIFIED_ADDRESS)
		}
		copy(e.mem[a:], m[6:6+n])
		return []byte{WRITE_DATA_BY_ADDRESS | 0x40}
	case WRITE_DATA_BY_LOCAL_IDENTIFIER:
		if m[0] < 2 || 1+int(m[0]) > len(m) {
			return nrc(SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT)
		}
		e.ids[m[2]] = append([]byte(nil), m[3:1+int(m[0])]...)
		return []byte{WRITE_DATA_BY_LOCAL_IDENTIFIER | 0x40, m[2]}
	case REQUEST_UPLOAD:
		if e.access != DEVELOPMENT_PRIORITY {
			return nrc(SECURITY_ACCESS_DENIED_OR_REQUESTED)
		}
		a, n := u24(m[2:]), u24(m[5:])
		if n == 0 || a+n > len(e.mem) {
			return nrc(CANNOT_UPLOAD_FROM_SPECIFIED_ADDRESS)
		}
		e.mode, e.upAddr, e.upLeft, e.upCount = REQUEST_UPLOAD, a, n, 0
		return []byte{REQUEST_UPLOAD | 0x40, 0xFF}
	case REQUEST_DOWNLOAD:
		a, n := u24(m[2:]), u24(m[6:])
		if a+n > len(e.flash) {
			return nrc(CANNOT_DOWNLOAD_TO_SPECIFIED_ADDRESS)
		}
		e.mode, e.dlAddr, e.dlEnd = REQUEST_DOWNLOAD, a, a+n
		return []byte{REQUEST_DOWNLOAD | 0x40}
	case TRANSFER_DATA:
		switch e.mode {
		case REQUEST_UPLOAD:
			return e.uploadBlock()
		case REQUEST_DOWNLOAD:
			if m[0] < 2 || 1+int(m[0]) > len(m) {
				return nrc(INCORRECT_BYTE_COUNT_DURING_BLOCK_TRANSFER)
			}
			data := m[2 : 1+int(m[0])]
			if e.dlAddr+len(data) > e.dlEnd {
				return nrc(INCORRECT_BYTE_COUNT_DURING_BLOCK_TRANSFER)
			}
			e.dlAddr += copy(e.flash[e.dlAddr:], data)
			return []byte{TRANSFER_DATA | 0x40}
		}
		return nrc(CANNOT_DOWNLOAD_TO_SPECIFIED_ADDRESS)
	case REQUEST_TRANSFER_EXIT:
		e.mode = 0
		return []byte{REQUEST_TRANSFER_EXIT | 0x40}
	case START_ROUTINE_BY_IDENTIFIER:
		return []byte{START_ROUTINE_BY_IDENTIFIER | 0x40, m[2]}
	case REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER:
		return []byte{REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER | 0x40, m[2], 0x00}
	case DYNAMICALLY_DEFINE_IDENTIFIER:
		return []byte{DYNAMICALLY_DEFINE_IDENTIFIER | 0x40, m[2]}
	}
	return nrc(SERVICE_NOT_SUPPORTED)
}

// uploadBlock is one transferData reply of an upload: mode, sub-function and
// block counter, then the data. Asking past the end is an incorrect byte count.
func (e *fakeT7) uploadBlock() []byte {
	if e.upLeft == 0 {
		return []byte{0x7F, TRANSFER_DATA, INCORRECT_BYTE_COUNT_DURING_BLOCK_TRANSFER}
	}
	n := min(e.upLeft, e.upBlock)
	p := append([]byte{TRANSFER_DATA | 0x40, REQUEST_UPLOAD, 0x00, e.upCount}, e.mem[e.upAddr:e.upAddr+n]...)
	e.upAddr, e.upLeft, e.upCount = e.upAddr+n, e.upLeft-n, e.upCount+1
	return p
}

func (e *fakeT7) securityAccess(m []byte) []byte {
	nrc := func(code byte) []byte { return []byte{0x7F, SECURITY_ACCESS, code} }
	if e.locked {
		return nrc(REQUIRED_TIME_DELAY_NOT_EXPIRED)
	}
	switch m[2] {
	case DEVELOPMENT_PRIORITY:
		if e.access == DEVELOPMENT_PRIORITY {
			return []byte{SECURITY_ACCESS | 0x40, m[2], 0x00, 0x00}
		}
		e.key = (e.seed<<2 ^ e.pair.XOR) - e.pair.Sub
		return []byte{SECURITY_ACCESS | 0x40, m[2], byte(e.seed >> 8), byte(e.seed)}
	case DEVELOPMENT_PRIORITY + 1:
		if k := uint16(m[3])<<8 | uint16(m[4]); k == e.key && k != 0 {
			e.access = DEVELOPMENT_PRIORITY
			return []byte{SECURITY_ACCESS | 0x40, m[2], SECURITY_ACCESS_ALLOWED}
		}
		if e.badKeys++; e.badKeys > 2 {
			e.badKeys, e.locked = 0, true
		}
		return nrc(SECURITY_ACCESS_DENIED_OR_REQUESTED)
	case HIGH_PRIORITY:
		return []byte{SECURITY_ACCESS | 0x40, m[2], byte(e.l3Seed >> 8), byte(e.l3Seed)}
	}
	return nrc(SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT)
}

// sids lists the service of every complete request the ECU received.
func (e *fakeT7) sids() []byte {
	var out []byte
	for _, m := range e.requests {
		out = append(out, m[1])
	}
	return out
}

func (e *fakeT7) count(sid byte) int {
	n := 0
	for _, s := range e.sids() {
		if s == sid {
			n++
		}
	}
	return n
}

// kwp is request i as the ECU reads it: the length byte and what it covers.
func (e *fakeT7) kwp(i int) []byte {
	m := e.requests[i]
	return m[:min(len(m), 1+int(m[0]))]
}

// sentOn lists the frames the tester sent on id.
func sentOn(node *gocantest.Node, id uint32) [][]byte {
	var out [][]byte
	for _, f := range node.Sent() {
		if f.ID == id {
			out = append(out, f.Bytes())
		}
	}
	return out
}

func shortTimeout(t *testing.T) {
	t.Helper()
	saved := DefaultTimeout
	DefaultTimeout = 30 * time.Millisecond
	t.Cleanup(func() { DefaultTimeout = saved })
}

// noWait skips the client's pauses between security access attempts and
// records the ones it would have taken.
func noWait(t *testing.T) *[]time.Duration {
	t.Helper()
	saved := wait
	var waits []time.Duration
	wait = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		return ctx.Err()
	}
	t.Cleanup(func() { wait = saved })
	return &waits
}

func answerWith(p ...byte) func([]byte) []byte {
	return func([]byte) []byte { return p }
}
