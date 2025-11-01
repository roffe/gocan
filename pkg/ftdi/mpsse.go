//go:build ftdi

package ftdi

import (
	"errors"
	"io"
	"time"
)

//import "fmt"

// MPSSE Commands
const (
	DISABLE_X5   = 0x8A
	ENABLE_X5    = 0x8B
	SET_CLK_DIV  = 0x86
	SET_LOW_BITS = 0x80
	SPI_0_WRITE  = 0x11
	SPI_0_READ   = 0x24
	SPI_0_TXRX   = 0x31
)

const CHUNK_SIZE = 1 << 16

type Spi struct {
	d         *Device
	io_dir    byte
	io_status byte
}

// Initialize an FTDI device for MPSSE mode
func InitializeSpi(d *Device) (s *Spi, e error) {
	s = &Spi{d, 0x0B, 0x08}
	d.Reset()
	d.Purge(FT_PURGE_RX | FT_PURGE_TX)
	d.SetTransferSize(CHUNK_SIZE, CHUNK_SIZE)
	d.SetChars(0, 0)
	//d.SetTimeouts(1000)
	d.SetLatency(2)
	d.SetFlowControl(RTS_CTS)
	d.SetBitMode(RESET)
	d.SetBitMode(MPSSE)

	// Sleep for setup
	time.Sleep(20 * time.Millisecond)

	// Send bad command to synchronize
	d.Write([]byte{0xAB})

	buf := make([]byte, 2)
	n, e := d.Read(buf)
	if n != 2 || buf[0] != 0xFA || buf[1] != 0xAB {
		return nil, errors.New("error synchronizing with MPSSE core")
	}

	initCmds := []byte{
		0x85, // Disable Loopback
		0x8A, // Disable divide-by-5 prescaler
		0x97, // Disable adaptive clock
		0x8D, // Disable three-phase clock
		0x80, // Set low-byte port
		0x08, // All initially low, except CS
		0x0B, // CLK, MISO, CS, output; others inputs
	}
	d.Write(initCmds)
	time.Sleep(20 * time.Millisecond)
	//	bb := make([]byte, 10)
	//	n, e = d.Read(bb)
	//fmt.Println(n, e, bb)

	return s, nil
}

func (s *Spi) setCS(cs byte) []byte {
	cmd := []byte{0x80, s.io_status, s.io_dir}
	if cs == 0 {
		cmd[1] &= 0xF7
	} else {
		cmd[1] |= 0x08
	}
	return cmd
}

func (s *Spi) Read(b []byte) (n int, e error) {
	if len(b) == 0 {
		return 0, nil
	}

	cmds := []byte{}

	// Set CS Low
	cmds = append(cmds, s.setCS(0)...)

	// clock in bytes a chunk at a time
	to_read := len(b)
	if len(b) > CHUNK_SIZE {
		to_read = CHUNK_SIZE
	}
	cmds = append(cmds, SPI_0_READ, low_byte(to_read-1), high_byte(to_read-1))
	cmds = append(cmds, b[0:to_read]...)

	// De-assert CS
	cmds = append(cmds, s.setCS(1)...)
	s.d.Write(cmds)
	n, e = io.ReadFull(s.d, b[:to_read])

	if n != to_read {
		return n, errors.New("failed to read all data")
	}

	if e != nil {
		return n, e
	}
	if to_read != len(b) {
		nn, e := s.Read(b[to_read:])
		return n + nn, e
	}
	return n, nil
}

func (s *Spi) Write(b []byte) (n int, e error) {
	if len(b) == 0 {
		return 0, nil
	}

	cmds := []byte{}

	// Set CS Low
	cmds = append(cmds, s.setCS(0)...)

	// clock out bytes a chunk at a time
	to_write := len(b)
	if len(b) > CHUNK_SIZE {
		to_write = CHUNK_SIZE
	}
	cmds = append(cmds, SPI_0_WRITE, low_byte(to_write-1), high_byte(to_write-1))
	cmds = append(cmds, b[0:to_write]...)

	// De-assert CS
	cmds = append(cmds, s.setCS(1)...)
	n, e = s.d.Write(cmds)

	if e != nil {
		return n, e
	}
	if to_write != len(b) {
		nn, e := s.Write(b[to_write:])
		return n + nn, e
	}
	return n, nil
}

// Write `w` bytes while also reading into `r`
func (s *Spi) Transfer(b []byte) (r []byte, e error) {
	if len(b) == 0 {
		return r, nil
	}

	cmds := []byte{}

	// Set CS Low
	cmds = append(cmds, s.setCS(0)...)

	// clock out bytes a chunk at a time
	to_write := len(b)
	if len(b) > CHUNK_SIZE {
		to_write = CHUNK_SIZE
	}
	cmds = append(cmds, SPI_0_TXRX, low_byte(to_write-1), high_byte(to_write-1))
	cmds = append(cmds, b[0:to_write]...)

	// De-assert CS
	cmds = append(cmds, s.setCS(1)...)
	s.d.Write(cmds)
	nr, e := io.ReadFull(s.d, cmds[:to_write])
	if nr != to_write {
		return r, errors.New("failed to read all data")
	}
	r = append(r, cmds[:nr]...)

	if e != nil {
		return r, e
	}

	if to_write != len(b) {
		rr, e := s.Transfer(b[to_write:])
		return append(r, rr...), e
	}
	return r, nil
}

func (s *Spi) SetClk(freq int) (e error) {
	var clk int
	cmds := []byte{}
	if freq > 3e6 {
		cmds = append(cmds, DISABLE_X5)
		clk = 60e6
	} else {
		cmds = append(cmds, ENABLE_X5)
		clk = 12e6
	}
	//Todo: this should round to the nearest
	divisor := ((clk / freq) / 2) - 1
	if divisor >= 1<<16 {
		divisor = 1<<16 - 1
	}
	cmds = append(cmds, 0x86, high_byte(divisor), low_byte(divisor))
	_, e = s.d.Write(cmds)
	return e
}

func (s *Spi) SetGPIO(outputs byte) (e error) {
	cmd := []byte{0x80, s.io_status, s.io_dir}
	if outputs == 0 {
		cmd[1] &= 0xF7
	} else {
		cmd[1] |= 0x08
	}
	//TODO
	return
}

func (s *Spi) WriteGPIO(state byte) (e error) {
	cmd := []byte{0x80, s.io_status, s.io_dir}
	if state == 0 {
		cmd[1] &= 0xF7
	} else {
		cmd[1] |= 0x08
	}
	//TODO
	return
}

func (s *Spi) ReadGPIO(state byte) (e error) {
	return
}

/*---------------------------------------------------------------------------*/
/*---------------------------------------------------------------------------*/
/*---------------------------------------------------------------------------*/
/*---------------------------------------------------------------------------*/

type BitBang struct {
	d *Device
}

func InitializeBitBang(d *Device) (s *BitBang, e error) {
	return
}

func (bb *BitBang) Read(b []byte) (n int, e error) {
	return
}

func (bb *BitBang) Write(b []byte) (n int, e error) {
	return
}

func high_byte(i int) byte {
	return byte((i & 0xFF00) >> 8)
}

func low_byte(i int) byte {
	return byte(i & 0x00FF)
}
