package gmlan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/roffe/gocan"
)

type Client struct {
	c *gocan.Client
}

func New(c *gocan.Client) *Client {
	return &Client{
		c: c,
	}
}

func (cl *Client) DisableNormalCommunication(ctx context.Context) error {
	if err := cl.c.SendFrame(0x101, []byte{0xFE, 0x01, 0x28}, gocan.Outgoing); err != nil { // DisableNormalCommunication Request Message
		return err
	}
	return nil
}

/* $A5
subFunc
01 requestProgrammingMode
  Request by the tester to verify the capability of the node(s) to enter into a normal speed
  programming event.

02 requestProgrammingMode_HighSpeed
  Request by the tester to verify the capability of the node(s) to enter into a high speed
  programming event.

03 enableProgrammingMode
  Request by the tester to have the node(s) enter into a programming event. This can only
  be sent if preceded by one of the valid requestProgrammingMode messages (above).
*/
func (cl *Client) ProgrammingMode(ctx context.Context, subFunc byte, canID, recvID uint32) error {
	payload := []byte{0x02, 0xA5, subFunc}
	switch subFunc {
	case 0x01, 0x02:
		frame := gocan.NewFrame(canID, payload, gocan.ResponseRequired)
		resp, err := cl.c.SendAndPoll(ctx, frame, 150*time.Millisecond, recvID)
		if err != nil {
			return err
		}
		d := resp.Data()
		if d[0] != 0x01 || d[1] != 0xE5 {
			return errors.New("request ProgrammingMode invalid response")
		}
		return nil

	case 0x03:
		frame := gocan.NewFrame(canID, payload, gocan.Outgoing)
		if err := cl.c.Send(frame); err != nil {
			return err
		}
		return nil
	default:
		return errors.New("ProgrammingMode: unknown subFunc")
	}
}

/* subFunc
02 disableAllDTCs
   This level shall disable setting of all DTCs.

03 enableDTCsDuringDevCntrl
   This level shall be used to allow DTC algorithms to continue to execute while the
   DeviceControl ($AE) service is active. This request shall have to be made prior to
   activating DeviceControl or the request shall be rejected. If this service and level are not
   requested before entering DeviceControl, then DTCs shall be inhibited while
   DeviceControl is active. (See the $AE service for further details).
   Note: If another diagnostic service is requested which disables DTCs (after the request is
   sent to allow DTCs to run during DeviceControl) then the DTCs shall become inhibited
   and remain inhibited until after a TesterPresent timeout occurs or a $20 service is
   requested.

04 wakeUpLinks
   This level shall cause a gateway ECU to initiate the appropriate wake-up sequence on all
   GMLAN subnets that it is connected to (provided that a given subnet has a wake-up
   mechanism defined).
   Note: The rules for sending a wake-up as defined in GMW 3104 - GMLAN
   Communications Strategy Specification still apply (e.g., the strategy specification restricts
   wake-up requests to have a minimum time interval between them. If a diagnostic request
   is received to initiate a wake-up and the minimum interval has not expired, then the ECU
   shall send the positive response message back to the tester without initiating another
   wake-up).
   If a GMLAN subnet uses a shared local input as a wake-up wire and the shared local
   input has to remain asserted to keep communications active, then the gateway device
   shall ensure that the wake-up wire is asserted while the gateways diagnostic VN is active.
   Note: An example of the shared local input wake-up mechanism described above would
   be a gateway that is connected to both the single wire CAN link and a dual wire CAN link.
   In this example, the gateway uses a relay to switch power to the other devices on the dual
   wire CAN subnet. For normal operations the gateway would receive the High Voltage
   wake-up on the single wire CAN bus and then enable the relay to provide power to the
   dual wire devices. If the ECU receives a request for this service with the wakeUpLinks
   ($04) sub-function parameter, then the ECU would ensure that the relay providing power
   to the dual wire link ECUs remains enabled as long as the diagnostic VN is active in the
   gateway (or longer if the ECU would otherwise keep t
*/
func (cl *Client) InitiateDiagnosticOperation(ctx context.Context, subFunc byte, canID, recvID uint32) error {
	payload := []byte{0x02, 0x10, subFunc}
	frame := gocan.NewFrame(canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, frame, 150*time.Millisecond, recvID)
	if err != nil {
		return errors.New("InitiateDiagnosticOperation: " + err.Error())
	}
	if err := CheckErr(resp); err != nil {
		return errors.New("InitiateDiagnosticOperation: " + err.Error())
	}

	d := resp.Data()
	if d[0] != 0x01 || d[1] != 0x50 {
		log.Println(resp.String())
		return errors.New("invalid response to InitiateDiagnosticOperation request")
	}

	return nil
}

// $A2
func (cl *Client) ReportProgrammedState(ctx context.Context, canID, recvID uint32) error {
	payload := []byte{0x01, 0xA2}
	frame := gocan.NewFrame(canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, frame, 150*time.Millisecond, recvID)
	if err != nil {
		return errors.New("ReportProgrammedState: " + err.Error())
	}
	if err := CheckErr(resp); err != nil {
		return errors.New("ReportProgrammedState: " + err.Error())
	}
	d := resp.Data()
	if d[0] != 0x02 || d[1] != 0xE2 {
		return errors.New("invalid response to ReportProgrammedState request")
	}
	return nil
}

func (cl *Client) WriteDataByIdentifier(ctx context.Context, canID, recvID uint32, pid byte, data []byte) error {
	r := bytes.NewReader(data)
	firstPart := make([]byte, 4)
	_, err := r.Read(firstPart)
	if err != nil {
		if err == io.EOF {
			// do nothing
		} else {
			return err
		}
	}
	payload := []byte{0x10, byte(len(data) + 2), 0x3B, pid}
	payload = append(payload, firstPart...)
	fr := gocan.NewFrame(canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, fr, 150*time.Millisecond, recvID)
	if err != nil {
		return err
	}
	d := resp.Data()
	if d[0] != 0x30 || d[1] != 0x00 {
		log.Println(resp.String())
		return errors.New("invalid response to initial writeDataByIdentifier")
	}

	delay := d[2]

	var seq byte = 0x21

	for r.Len() > 0 {
		pkg := []byte{seq}
	inner:
		for i := 1; i < 8; i++ {
			b, err := r.ReadByte()
			if err != nil {
				if err == io.EOF {
					log.Println("eof")
					break inner
				}
				return err
			}
			pkg = append(pkg, b)
		}
		cl.c.SendFrame(canID, pkg, gocan.Outgoing)
		log.Printf("%X\n", pkg)
		time.Sleep(time.Duration(delay) * time.Millisecond)
		seq++
		if seq == 0x30 {
			seq = 0x20
		}
	}

	return nil
}

// $1A
func (cl *Client) ReadDataByIdentifier(ctx context.Context, canID, recvID uint32, pid byte) ([]byte, error) {
	out := bytes.NewBuffer([]byte{})
	//resp, err := cl.c.Poll(ctx, 150*time.Millisecond, recvID) // +0x400?
	f := gocan.NewFrame(canID, []byte{0x02, 0x1A, pid}, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, f, 150*time.Millisecond, recvID)
	if err != nil {
		return nil, err
	}
	if err := CheckErr(resp); err != nil {
		return nil, err
	}
	d := resp.Data()
	switch {
	case d[1] == 0x5A: // only one frame in this response
		//out := bytes.NewBuffer(nil)
		length := d[0]
		for i := 3; i <= int(length); i++ {
			out.WriteByte(d[i])
		}
		return out.Bytes(), nil
	case d[2] == 0x5A:
		//out := bytes.NewBuffer(nil)
		leng := d[1]
		lenThisFrame := int(leng)
		left := int(leng)
		framesToReceive := (leng - 4) / 8
		if (leng-4)%8 > 0 {
			framesToReceive++
		}
		if lenThisFrame > 4 {
			lenThisFrame = 4
		}
		for i := 4; i < 4+lenThisFrame; i++ {
			out.WriteByte(d[i])
			left--
		}

		cl.c.SendFrame(0x7E0, []byte{0x30, 0x00}, gocan.CANFrameType{Type: 2, Responses: int(framesToReceive)})

		var seq byte = 0x21

	outer:
		for framesToReceive > 0 {
			resp2, err := cl.c.Poll(ctx, 150*time.Millisecond, recvID)
			if err != nil {
				return nil, err
			}
			if err := CheckErr(resp2); err != nil {
				return nil, err
			}
			d2 := resp2.Data()
			if d2[0] != seq {
				return nil, fmt.Errorf("frame sequence out of order, expected 0x%X got 0x%X", seq, d2[0])
			}

			for i := 1; i < 8; i++ {
				out.WriteByte(d2[i])
				left--
				if left == 0 {
					break outer
				}
			}
			framesToReceive--
			seq++
			if seq == 0x30 {
				seq = 0x20
			}
		}
		return out.Bytes(), nil
	}
	log.Println(resp.String())
	return nil, errors.New("unhandled response")
}

// $AA
func (cl *Client) ReadDataByPacketIdentifier(ctx context.Context, canID, recvID uint32, subFunc byte, dpid ...byte) ([]byte, error) {
	f := gocan.NewFrame(
		canID,
		append(
			[]byte{byte(len(dpid) + 2), 0xAA, subFunc},
			dpid...,
		),
		gocan.CANFrameType{
			Type:      2,
			Responses: len(dpid),
		},
	)
	resp, err := cl.c.SendAndPoll(ctx, f, 150*time.Millisecond, recvID)
	if err != nil {
		return nil, err
	}

	if err := CheckErr(resp); err != nil {
		return nil, err
	}

	return resp.Data(), nil
}

// $34
func (cl *Client) RequestDownload(ctx context.Context, canID, recvID uint32, z22se bool) error {
	payload := []byte{0x06, 0x34, 0x00, 0x00, 0x00, 0x00, 0x00}

	if z22se {
		payload[0] = 0x05
	}

	f := gocan.NewFrame(canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, f, 150*time.Millisecond, recvID)
	if err != nil {
		return err
	}

	if err := CheckErr(resp); err != nil {
		return errors.New("RequestDownload: " + err.Error())
	}

	d := resp.Data()
	if d[0] != 0x01 || d[1] != 0x74 {
		return errors.New("/!\\ Did not receive correct response from RequestDownload")
	}

	return nil
}

func (cl *Client) TransferData(ctx context.Context, length byte, startAddress int, canID, responseID uint32) error {
	payload := []byte{0x10, length, 0x36, 0x00, 0x00, 0x00, 0x00, 0x00}
	payload[7] = byte(startAddress)
	payload[6] = byte(startAddress >> 8)
	payload[5] = byte(startAddress >> 16)

	f := gocan.NewFrame(canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, f, 150*time.Millisecond, responseID)
	if err != nil {
		return err
	}

	if err := CheckErr(resp); err != nil {
		return errors.New("TransferData: " + err.Error())
	}

	d := resp.Data()
	if d[0] != 0x30 || d[1] != 0x00 {
		return errors.New("/!\\ Did not receive correct response from TransferData")
	}

	return nil
}
