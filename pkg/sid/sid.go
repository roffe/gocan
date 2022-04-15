package sid

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/avast/retry-go"
	"github.com/roffe/gocan"
)

type Client struct {
	//In             chan string
	dbuff          []byte
	c              *gocan.Client
	defaultTimeout time.Duration
	sync.Mutex
}

func New(c *gocan.Client) *Client {
	dbuff := make([]byte, 4)

	// Preload the first value into the array/slice
	dbuff[0] = 0x20

	// Incrementally duplicate the value into the rest of the container
	for j := 1; j < len(dbuff); j *= 2 {
		copy(dbuff[j:], dbuff[:j])
	}

	return &Client{
		//In:             make(chan string, 10),
		dbuff:          dbuff,
		c:              c,
		defaultTimeout: 150 * time.Millisecond,
	}
}

func (t *Client) In(b []byte) {
	t.Lock()
	defer t.Unlock()
	t.dbuff = b
}

func (t *Client) StartRadioDisplay(ctx context.Context) error {
	_, err := t.RequestAccess(ctx)
	if err != nil {
		return err
	}
	go t.radio(ctx)
	return nil
}

func (t *Client) radio(ctx context.Context) {
	//	tc := time.NewTicker(130 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			//log.Println(string(t.dbuff))
			t.Lock()
			if err := t.SetRadioText(t.dbuff); err != nil {
				log.Println(err)
				t.Unlock()
				return
			}
			t.Unlock()
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (t *Client) Beep() error {
	return t.c.SendFrame(0x430, []byte{0x80, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, gocan.Outgoing)
}

func (t *Client) RequestAccess(ctx context.Context) ([]byte, error) {

	// [1] FF=inget meddelande, prio.nivå
	// [2] request type: 1 = Engineering test; 2 = Emergency; 3 = Driver action; 4 = ECU action; 5 = Static text; 0xFF = We don't want to write on SID
	// [3] 19=IHU 12=SPA 23=ACC

	req := gocan.NewFrame(0x348, []byte{0x11, 0x02, 0x05, 0x19, 0x05, 0x00, 0x00, 0x00}, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, req, 200*time.Millisecond, 0x368)
	if err != nil {
		return nil, err
	}

	return resp.Data(), nil
}

func (t *Client) SetRadioText(text []byte) error {
	row := 2
	r := bytes.NewReader(text)
	pkgs := len(text) / 5
	seq := 0x40 + pkgs
	first := []byte{byte(seq), 0x96, byte(row)}
	for i := 0; i < 5; i++ {
		bb, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				bb = 0x00
			} else {
				return err
			}
		}
		first = append(first, bb)
	}
	ff := gocan.NewFrame(0x328, first, gocan.Outgoing)
	t.c.Send(ff)
	seq -= 0x41

	for seq >= 0 {
		asd := make([]byte, 5)
		_, err := r.Read(asd)
		if err != nil {
			if err != io.EOF {
				return err
			}
		}
		data := append([]byte{byte(seq), 0x96, byte(row)}, asd...)
		frame := gocan.NewFrame(0x328, data, gocan.Outgoing)
		t.c.Send(frame)
		seq--
	}
	return nil
}

func Translate(str string) []byte {
	var out bytes.Buffer
	for _, c := range str {
		switch c {
		case 'å':
			out.WriteByte(16)
		case 'Å':
			out.WriteByte(225)
		case 'ä':
			out.WriteByte(17)
		case 'Ä':
			out.WriteByte(209)
		case 'ö':
			out.WriteByte(18)
		case 'Ö':
			out.WriteByte(215)
		default:
			out.WriteRune(c)
		}

	}
	return out.Bytes()
}

func (t *Client) StartDiagnosticSession(ctx context.Context) error {
	log.Println("starting diagnostics session")
	testerID := 0x240

	err := retry.Do(
		func() error {
			data := []byte{0x3F, 0x81, 0x00, 0x65, byte((testerID >> 8) & 0xFF), byte(testerID & 0xFF), 0x00, 0x00}
			t.c.SendFrame(0x220, data, gocan.ResponseRequired)
			f, err := t.c.Poll(ctx, t.defaultTimeout, 0x22B)
			if err != nil {
				return err
			}
			d := f.Data()
			if d[0] == 0x40 && d[3] == 0xC1 {
				log.Printf("Tester address: 0x%X\n", d[1])
				log.Printf("ECU address: 0x%X\n", d[2]|0x80)
				log.Printf("ECU ID: 0x%X", uint16(d[6])<<8|uint16(d[7]))
				return nil
			}

			return fmt.Errorf("invalid response to enter diagnostics session: %s", f.String())
		},
		retry.Context(ctx),
		retry.Attempts(5),
		retry.OnRetry(func(n uint, err error) {
			log.Println(err)
		}),
	)
	if err != nil {
		return err
	}

	return nil
}

// Set the RDS status icons at the bottom of the SID display
func (t *Client) SetRDSStatus(noRDS, rds, noTP, tp, reg, pty bool) error {
	var out byte
	if rds {
		out += 1 << 7
	}
	if noTP {
		out += 1 << 6
	}
	if reg {
		out += 1 << 5
	}
	if tp {
		out += 1 << 4
	}
	if pty {
		out += 1 << 3
	}
	if noRDS {
		out += 1
	}
	return t.c.SendFrame(0x380, []byte{0x0F, 0x00, out, 0x00, 0x00, 0x00, 0x00, 0x00}, gocan.Outgoing)
}
