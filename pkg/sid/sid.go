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
	err := t.SIDAudioTextControl(ctx, 2, 5, 0x19)
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

/*
	Format of NODE_DISPLAY_RESOURCE_REQ frame:

ID: Node ID requesting to write on SID
[0]: Request source
[1]: SID object to write on; 0 = entire SID; 1 = 1st row; 2 = 2nd row
[2]: Request type: 1 = Engineering test; 2 = Emergency; 3 = Driver action; 4 = ECU action; 5 = Static text; 0xFF = We don't want to write on SID
[3]: Request source function ID 19=IHU 12=SPA 23=ACC 14=SID
[4-7]: Zeroed out; not in use
*/
func (t *Client) SIDAudioTextControl(ctx context.Context, row, status, from byte) error {
	var q byte = 0x00
	req := gocan.NewFrame(0x348, []byte{0x1F, row, status, from, q, 0x00, 0x00, 0x00}, gocan.Outgoing)
	err := t.c.Send(req)
	//resp, err := t.c.SendAndPoll(ctx, req, 200*time.Millisecond, 0x368)
	if err != nil {
		return err
	}
	log.Println(req.String())
	return nil
}

func (t *Client) SetRadioText(text []byte) error {
	row := 2
	r := bytes.NewReader(text)
	pkgs := len(text) / 5
	seq := 0x40 + pkgs
	firstPkg := true
	for seq >= 0 {
		pkg := []byte{byte(seq), 0x96, byte(row)}
		for i := 0; i < 5; i++ {
			bb, err := r.ReadByte()
			if err != nil {
				if err == io.EOF {
					bb = 0x00
				} else {
					return err
				}
			}
			pkg = append(pkg, bb)
		}
		frame := gocan.NewFrame(0x328, pkg, gocan.Outgoing)
		t.c.Send(frame)
		log.Println(frame.String())
		if firstPkg {
			seq -= 0x41
			firstPkg = false
		} else {
			seq--
		}

	}
	return nil
}

func (t *Client) SetFullScreen(row1, row2 string) error {

	return nil
}

func (t *Client) SetText(text []byte, row int) error {
	r := bytes.NewReader(text)
	pkgs := len(text) / 5
	seq := 0x40 + pkgs
	firstPkg := true
	for seq >= 0 {
		pkg := []byte{byte(seq), 0x96, byte(row)}
		for i := 0; i < 5; i++ {
			bb, err := r.ReadByte()
			if err != nil {
				if err == io.EOF {
					bb = 0x00
				} else {
					return err
				}
			}
			pkg = append(pkg, bb)
		}
		frame := gocan.NewFrame(0x328, pkg, gocan.Outgoing)
		t.c.Send(frame)
		log.Println(frame.String())
		if firstPkg {
			seq -= 0x41
			firstPkg = false
		} else {
			seq--
		}

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
