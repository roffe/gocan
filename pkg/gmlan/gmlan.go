// see GMW3110 gmw3110-2010.pdf
package gmlan

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/roffe/gocan"
)

type GMLanOption func(*Client)

type Client struct {
	c              *gocan.Client
	defaultTimeout time.Duration
	canID          uint32
	recvID         []uint32
	//closed         chan struct{}
}

const (
	TRANSFER_DATA                  = 0x36
	DEVICE_CONTROL                 = 0xAE
	DISABLE_NORMAL_COMMUNICATION   = 0x28
	INITIATE_DIAGNOSTIC_OPERATION  = 0x10
	PROGRAMMING_MODE               = 0xA5
	RETURN_TO_NORMAL_MODE          = 0x20
	REPORT_PROGRAMMED_STATE        = 0xA2
	READ_DATA_BY_IDENTIFIER        = 0x1A
	READ_DATA_BY_PACKET_IDENTIFIER = 0xAA
	SECURITY_ACCESS                = 0x27
	REQUEST_DOWNLOAD               = 0x34
	WRITE_DATA_BY_IDENTIFIER       = 0x3B
	DYNAMICALLY_DEFINE_MESSAGE     = 0x2C
	READ_MEMORY_BY_ADDRESS         = 0x23
)

func New(c *gocan.Client, canID uint32, recvID ...uint32) *Client {
	//return &Client{
	//	c:              c,
	//	defaultTimeout: 150 * time.Millisecond,
	//	canID:          canID,
	//	recvID:         recvID,
	//}
	return NewWithOpts(c, WithCanID(canID), WithRecvID(recvID...))
}

func NewWithOpts(client *gocan.Client, opts ...GMLanOption) *Client {
	c := newDefault(client)
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func newDefault(client *gocan.Client) *Client {
	return &Client{
		c:              client,
		defaultTimeout: 200 * time.Millisecond,
	}
}

func WithDefaultTimeout(timeout time.Duration) GMLanOption {
	return func(c *Client) {
		c.defaultTimeout = timeout
	}
}

func WithCanID(canID uint32) GMLanOption {
	return func(c *Client) {
		c.canID = canID
	}
}

func WithRecvID(recvID ...uint32) GMLanOption {
	return func(c *Client) {
		c.recvID = recvID
	}
}

// 8.2 InitiateDiagnosticOperation ($10) Service.
/*
This service allows the tester to perform the following tasks:
* Disable the setting of all DTCs while the tool continues to perform other diagnostic services.
* Allow ECU DTC algorithms to continue to execute while the DeviceControl ($AE) service is active.
* Request a gateway ECU to issue a wake-up request.

- 02 disableAllDTCs
   This level shall disable setting of all DTCs.

- 03 enableDTCsDuringDevCntrl
   This level shall be used to allow DTC algorithms to continue to execute while the
   DeviceControl ($AE) service is active. This request shall have to be made prior to
   activating DeviceControl or the request shall be rejected. If this service and level are not
   requested before entering DeviceControl, then DTCs shall be inhibited while
   DeviceControl is active. (See the $AE service for further details).
   Note: If another diagnostic service is requested which disables DTCs (after the request is
   sent to allow DTCs to run during DeviceControl) then the DTCs shall become inhibited
   and remain inhibited until after a TesterPresent timeout occurs or a $20 service is
   requested.

- 04 wakeUpLinks
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
func (cl *Client) InitiateDiagnosticOperation(ctx context.Context, subFunc byte) error {
	frame := &gocan.CANFrame{
		Identifier: cl.canID,
		Data:       []byte{0x02, INITIATE_DIAGNOSTIC_OPERATION, subFunc},
		FrameType:  gocan.ResponseRequired,
	}
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("InitiateDiagnosticOperation[1]: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		return fmt.Errorf("InitiateDiagnosticOperation[2]: %w", err)
	}
	if resp.Data[0] != 0x01 || resp.Data[1] != 0x50 {
		log.Println(resp.String())
		return errors.New("InitiateDiagnosticOperation[3]: invalid response")
	}
	return nil
}

// 8.4 ReadDataByIdentifier ($1A) Service.
//
// The purpose of this service is to provide the ability to read the
// content of pre-defined ECU data referenced by a dataIdentifier (DID) which contains static information such as
// ECU identification data or other information which does not require real-time updates. (Real-time data is
// intended to be retrieved via the ReadDataByPacketIdentifier ($AA) service.)
func (cl *Client) ReadDataByIdentifierUint16(ctx context.Context, pid byte) (uint16, error) {
	resp, err := cl.ReadDataByIdentifier(ctx, pid)
	if err != nil {
		return 0, err
	}
	retval := uint16(resp[0]) * 256
	retval += uint16(resp[1])
	return retval, nil
}

func (cl *Client) ReadDataByIdentifierString(ctx context.Context, pid byte) (string, error) {
	resp, err := cl.ReadDataByIdentifier(ctx, pid)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(resp[:]), "\x00", ""), nil
}

func (cl *Client) ReadDataByIdentifier(ctx context.Context, pid byte) ([]byte, error) {
	readFrame := &gocan.CANFrame{
		Identifier: cl.canID,
		Data:       []byte{0x02, READ_DATA_BY_IDENTIFIER, pid},
		FrameType:  gocan.ResponseRequired,
	}
	return cl.ReadDataByIdentifierFrame(ctx, readFrame)
}

func (cl *Client) ReadDataByIdentifierFrame(ctx context.Context, frame *gocan.CANFrame) ([]byte, error) {
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return nil, fmt.Errorf("ReadDataByIdentifier[1]: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		log.Println(resp.String())
		return nil, fmt.Errorf("ReadDataByIdentifier[2]: %w", err)
	}
	d := resp.Data
	if len(d) == 8 && bytes.Equal(d, []byte{0x01, 0x60, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) {
		return nil, fmt.Errorf("ReadDataByIdentifier[3]: busy, try again")
	}
	if len(d) >= 4 && bytes.HasPrefix(d, []byte{0x02, 0x1A, 0x18, 0x00}) {
		return nil, fmt.Errorf("ReadDataByIdentifier[4]: busy, try again")
	}
	// Single-frame positive response (len, 0x5A, DID, data...)
	if len(d) >= 3 && d[1] == 0x5A {
		payloadLen := int(d[0]) - 2 // exclude SID+ID
		start := 3
		end := start + payloadLen
		if payloadLen < 0 || end > len(d) {
			return nil, errors.New("ReadDataByIdentifier[5]: malformed single-frame response")
		}
		return d[start:end], nil
	}
	// First frame of a multi-frame response
	if len(d) >= 5 && d[0] == 0x10 {
		total := int(d[1]) - 2 // bytes to return (exclude SID+ID)
		if total < 0 {
			return nil, errors.New("ReadDataByIdentifier[6]: negative payload length in first frame")
		}
		buf := make([]byte, total)
		w := copy(buf, d[4:]) // up to 4 bytes already in first frame
		left := max(0, total-w)
		// How many consecutive frames we expect (7 data bytes per CF)
		framesToReceive := (left + 6) / 7
		sub := cl.c.Subscribe(ctx, cl.recvID...)
		defer sub.Close()
		// Send Flow Control: Continue to send (CTS), no block size, no separation time
		fc := &gocan.CANFrame{
			Identifier: cl.canID,
			Data:       []byte{0x30, 0x00, 0x00},
			FrameType:  gocan.CANFrameType{Type: 2, Responses: int(framesToReceive)},
		}
		if err := cl.c.SendFrame(fc); err != nil {
			return nil, fmt.Errorf("ReadDataByIdentifier[7]: %w", err)
		}
		// Expect consecutive frames starting at 0x21 .. 0x2F, then wrap to 0x20, etc.
		seq := byte(0x21)
		// Reusable timer (less GC churn than time.After in a loop)
		timer := time.NewTimer(cl.defaultTimeout)
		dataChan := sub.Chan()
		for framesToReceive > 0 {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(cl.defaultTimeout)
			select {
			case response := <-dataChan:
				fd := response.Data
				if len(fd) < 2 {
					return nil, fmt.Errorf("ReadDataByIdentifier[8]: short consecutive frame")
				}
				// If not a CF (0x2n), check for error frame
				if (fd[0] & 0x20) != 0x20 {
					if err := CheckErr(response); err != nil {
						return nil, fmt.Errorf("ReadDataByIdentifier[9]: %w", err)
					}
				}
				if fd[0] != seq {
					return nil, fmt.Errorf("ReadDataByIdentifier[10]: frame sequence out of order, expected 0x%X got 0x%X", seq, fd[0])
				}
				n := copy(buf[w:], fd[1:]) // copy up to 7 bytes
				w += n
				left -= n
				if left < 0 {
					left = 0
				}
				// Wrap sequence number without branches: 0x21..0x2F -> 0x20 -> 0x21...
				seq = 0x20 | ((seq + 1) & 0x0F)
				framesToReceive--
			case <-timer.C:
				return nil, fmt.Errorf("ReadDataByIdentifier[11]: timeout waiting for multi-frame response")
			}
		}
		return buf, nil
	}
	return nil, fmt.Errorf("ReadDataByIdentifier[12]: unknown response [% 02X]", resp.Data)
}

/*
8.5 ReturnToNormalMode ($20) Service. The purpose of this service is to return a node or group of nodes to
normal mode operation by canceling all active diagnostic services and resetting normal message
communications (if they were interrupted by a diagnostic operation).
All nodes participating in a GMLAN network shall support this service even if the node itself is diagnosed over
another vehicle bus (e.g., KWP2000 or Class 2). This requirement is necessary to facilitate programming of
other devices on the GMLAN subnet.
*/
func (cl *Client) ReturnToNormalMode(ctx context.Context) error {
	frame := &gocan.CANFrame{
		Identifier: cl.canID,
		Data:       []byte{0x01, RETURN_TO_NORMAL_MODE},
		FrameType:  gocan.ResponseRequired,
	}
	resp, err := cl.c.SendAndWait(
		ctx,
		frame,
		cl.defaultTimeout,
		cl.recvID...,
	)
	if err != nil {
		return fmt.Errorf("ReturnToNormalMode[1]: %w", err)
	}
	return CheckErr(resp)
}

/*
8.7 ReadMemoryByAddress ($23) Service. The purpose of this service is to retrieve data from a contiguous
range of ECU memory addresses. The range of ECU addresses is defined by a starting memory address
parameter and a length (memory size) parameter included in the request message. This service is intended to
be used during a device’s development cycle to allow access to data that may not be available via another
diagnostic service. The ReadMemoryByAddress service is only available as a one shot request-response
service
*/

func (cl *Client) ReadMemoryByAddress(ctx context.Context, address, length uint32) ([]byte, error) {
	// request
	frame := &gocan.CANFrame{
		Identifier: cl.canID,
		Data:       []byte{0x06, READ_MEMORY_BY_ADDRESS, byte(address >> 16), byte(address >> 8), byte(address), byte(length >> 8), byte(length)},
		FrameType:  gocan.ResponseRequired,
	}
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return nil, fmt.Errorf("ReadMemoryByAddress[1]: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		return nil, fmt.Errorf("ReadMemoryByAddress[2]: %w", err)
	}
	d := resp.Data

	// single-frame positive response: [len][SID+0x40][addrH][addrM][addrL][data...]
	if len(d) >= 5 && d[1] == READ_MEMORY_BY_ADDRESS+0x40 && d[0] < 0x10 {
		want := int(length)
		if want < 0 {
			return nil, errors.New("ReadMemoryByAddress[3]: invalid requested length")
		}
		start := 5
		end := start + want
		if end > len(d) {
			return nil, errors.New("ReadMemoryByAddress[4]: malformed single-frame response (too short)")
		}
		return d[start:end], nil
	}

	// first frame (multi-frame): [0x10][totalLen][SID+0x40][addrH][addrM][addrL][data0][data1]
	if len(d) >= 7 && d[0] == 0x10 && d[2] == READ_MEMORY_BY_ADDRESS+0x40 {
		// According to your original logic: effective data bytes = totalLen - 4
		// (excluding SID+addr(3)). Keep behavior but clamp with requested length.
		totalFF := int(d[1]) - 4
		total := min(int(length), max(0, totalFF))

		buf := make([]byte, total)
		w := copy(buf, d[6:min(6+2, len(d))]) // up to 2 bytes already in the FF
		left := max(0, total-w)

		framesToReceive := (left + 6) / 7 // ceil(left/7)

		// Subscribe before FC to avoid missing CFs
		sub := cl.c.Subscribe(ctx, cl.recvID...)
		defer sub.Close()

		// FC: CTS, BS=0, STmin=0
		fc := &gocan.CANFrame{
			Identifier: cl.canID,
			Data:       []byte{0x30, 0x00, 0x00},
			FrameType:  gocan.CANFrameType{Type: 2, Responses: framesToReceive},
		}
		if err := cl.c.SendFrame(fc); err != nil {
			return nil, fmt.Errorf("ReadMemoryByAddress[4]: %w", err)
		}

		seq := byte(0x21)
		timer := time.NewTimer(cl.defaultTimeout)
		dataChan := sub.Chan()
		for framesToReceive > 0 {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(cl.defaultTimeout)

			select {
			case r := <-dataChan:
				fd := r.Data
				if len(fd) < 2 {
					return nil, errors.New("ReadMemoryByAddress[5]: short consecutive frame")
				}
				// if not CF (0x2n), validate as error frame
				if (fd[0] & 0x20) != 0x20 {
					if err := CheckErr(r); err != nil {
						return nil, err
					}
				}
				if fd[0] != seq {
					return nil, fmt.Errorf("ReadMemoryByAddress[6]: frame sequence out of order, expected 0x%X got 0x%X", seq, fd[0])
				}

				n := copy(buf[w:], fd[1:]) // up to 7 bytes
				w += n
				left = max(0, left-n)

				// wrap 0x21..0x2F -> 0x20 -> 0x21...
				seq = 0x20 | ((seq + 1) & 0x0F)
				framesToReceive--

			case <-timer.C:
				return nil, errors.New("ReadMemoryByAddress[7]: timeout waiting for response")
			}
		}
		return buf, nil
	}
	return nil, errors.New("ReadMemoryByAddress[8]: unhandled response")
}

/*
func (cl *Client) ReadMemoryByAddress2(ctx context.Context, address, length uint32) ([]byte, error) {
	//log.Printf("ReadMemoryByAddress: address: %X, length: %X", address, length)
	frame := &gocan.CANFrame{
		Identifier: cl.canID,
		Data:       []byte{0x06, READ_MEMORY_BY_ADDRESS, byte(address >> 16), byte(address >> 8), byte(address), byte(length >> 8), byte(length)},
		FrameType:  gocan.ResponseRequired,
	}
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout*2, cl.recvID...)
	if err != nil {
		return nil, fmt.Errorf("ReadMemoryByAddress[1]: %w", err)
	}

	if err := CheckErr(resp); err != nil {
		return nil, fmt.Errorf("ReadMemoryByAddress[2]: %w", err)
	}
	switch {
	//case d[0] == 0x02 && d[1] == 0x1A && d[2] == frame.Data()[2]:
	//	return nil, fmt.Errorf("ReadDataByIdentifier: no more data")
	case resp.Data[1] == READ_MEMORY_BY_ADDRESS+0x40: // only one frame in this response
		return resp.Data[5 : 5+length], nil
	case resp.Data[0] == 0x10 && resp.Data[2] == READ_MEMORY_BY_ADDRESS+0x40: // Multi frame response
		left := int(resp.Data[1]) - 4
		out := bytes.NewBuffer(make([]byte, 0, left))
		rb := min(2, left)
		out.Write(resp.Data[6 : 6+rb])
		left -= rb

		framesToReceive := math.Ceil(float64(left) / 7)
		if err := cl.c.Send(cl.canID, []byte{0x30, 0x00, 0x00}, gocan.CANFrameType{Type: 2, Responses: int(framesToReceive)}); err != nil {
			return nil, fmt.Errorf("ReadMemoryByAddress[3]: %w", err)
		}

		cc := cl.c.Subscribe(ctx, cl.recvID...)
		defer cc.Close()
		var seq byte = 0x21
		for framesToReceive > 0 {
			select {
			case response := <-cc.Chan():
				frameData := response.Data
				if frameData[0]&0x20 != 0x20 {
					if err := CheckErr(response); err != nil {
						log.Println(response.String())
						return nil, err
					}
				}
				if frameData[0] != seq {
					return nil, fmt.Errorf("frame sequence out of order, expected 0x%X got 0x%X", seq, frameData[0])
				}
				rb = min(7, left)
				out.Write(frameData[1 : 1+rb])
				left -= rb
				if seq == 0x2F {
					seq = 0x1F //was 19?
				}
				seq++
				framesToReceive--
			case <-time.After(140 * time.Millisecond):
				return nil, errors.New("timeout waiting for response")
			}
		}
		return out.Bytes(), nil
	default:
		log.Println(resp.String())
		return nil, errors.New("unhandled response")
	}
}
*/

//func (cl *Client) sendAndReceive(ctx context.Context, payload []byte) (gocan.CANFrame, error) {
//	frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
//	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
//	if err != nil {
//		return nil, err
//	}
//	return resp, CheckErr(resp)
//}

// 8.8 SecurityAccess ($27) Service.
//
// The purpose of this service is to provide a means to access data and/or
// diagnostic services which have restricted access for security, emissions, or safety reasons. Diagnostic modes
// for downloading routines or data into a node and reading specific memory locations from a node are situations
// where security access may be required. Improper routines or data downloaded into a node could potentially
// damage the electronics or other vehicle components or risk the vehicle’s compliance to emission, safety, or
// security standards. This mode is intended
func (cl *Client) SecurityAccessRequestSeed(ctx context.Context, accessLevel byte) ([]byte, error) {
	frame := &gocan.CANFrame{
		Identifier: cl.canID,
		Data:       []byte{0x02, SECURITY_ACCESS, accessLevel},
		FrameType:  gocan.ResponseRequired,
	}
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return nil, fmt.Errorf("SecurityAccessRequestSeed[1]: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		return nil, fmt.Errorf("SecurityAccessRequestSeed[2]: %w", err)
	}
	if resp.Data[1] != 0x67 || resp.Data[2] != accessLevel {
		return nil, errors.New("SecurityAccessRequestSeed[3]: invalid response")
	}
	return []byte{resp.Data[3], resp.Data[4]}, nil
}

func (cl *Client) SecurityAccessSendKey(ctx context.Context, accessLevel, high, low byte) error {
	frame := &gocan.CANFrame{
		Identifier: cl.canID,
		Data:       []byte{0x04, SECURITY_ACCESS, accessLevel + 0x01, high, low},
		FrameType:  gocan.ResponseRequired,
	}
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("SecurityAccessSendKey[1]: %w", err)
	}

	if err := CheckErr(resp); err != nil {
		return fmt.Errorf("SecurityAccessSendKey[2]: %w", err)
	}

	if resp.Data[1] == 0x67 && resp.Data[2] == accessLevel+0x01 {
		//log.Println("Security access granted")
		return nil
	}
	return errors.New("SecurityAccessSendKey[3]: failed to obtain security access")
}

func (cl *Client) RequestSecurityAccess(ctx context.Context, accesslevel byte, delay time.Duration, seedfunc func([]byte, byte) (byte, byte)) error {
	seed, err := cl.SecurityAccessRequestSeed(ctx, accesslevel)
	if err != nil {
		return err
	}

	if seed[0] == 0x00 && seed[1] == 0x00 {
		//log.Println("Security access already granted")
		return nil
	}

	secondsToWait := delay.Milliseconds() / 1000
	for secondsToWait > 0 {
		time.Sleep(1 * time.Second)
		cl.TesterPresentNoResponseAllowed()
		secondsToWait--
	}

	high, low := seedfunc(seed, accesslevel)

	// log.Println("SecurityAccessSendKey")
	if err := cl.SecurityAccessSendKey(ctx, accesslevel, high, low); err != nil {
		return err
	}
	time.Sleep(45 * time.Millisecond)
	return nil
}

//8.9 DisableNormalCommunication ($28) Service.
/*
The purpose of this service is to prevent a device from
transmitting or receiving all messages which are not the direct result of a diagnostic request. The primary use
of the service is to set up a programming event. This is a required service that must be supported by all nodes
*/
func (cl *Client) DisableNormalCommunication(ctx context.Context) error {
	frame := &gocan.CANFrame{
		Identifier: cl.canID,
		Data:       []byte{0x01, DISABLE_NORMAL_COMMUNICATION},
		FrameType:  gocan.ResponseRequired,
	}
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("DisableNormalCommunication: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		return err
	}
	if resp.Data[0] != 0x01 || resp.Data[1] != 0x68 {
		return errors.New("invalid response to DisableNormalCommunication")
	}
	return nil
}

// AllNodes functional diagnostic request CANId ($101) and the AllNodes extended address ($FE).
func (cl *Client) DisableNormalCommunicationAllNodes() error {
	if err := cl.c.Send(0x101, []byte{0xFE, 0x01, DISABLE_NORMAL_COMMUNICATION}, gocan.Outgoing); err != nil {
		return err
	}
	return nil
}

/*
8.10 DynamicallyDefineMessage ($2C) Service. This service is used to dynamically define the contents of
diagnostic data packets which are formatted as UUDT messages and can be requested via the
ReadDataByPacketIdentifier ($AA) service. The use of dynamic data packets allows a test device to optimize
its diagnostic routines and bus bandwidth utilization by packing
*/
func (cl *Client) DynamicallyDefineMessage(ctx context.Context, ids ...uint16) error {
	frame := &gocan.CANFrame{
		Identifier: cl.canID,
		Data:       []byte{0x06, DYNAMICALLY_DEFINE_MESSAGE, 0xFE, 0x03, 0x8D, 0x01, 0x01},
		FrameType:  gocan.ResponseRequired,
	}
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("DynamicallyDefineMessage[1]: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		return fmt.Errorf("DynamicallyDefineMessage[2]: %w", err)
	}
	return nil
}

// 8.12 RequestDownload ($34) Service. This service is used in order to prepare a node to be programmed
func (cl *Client) RequestDownload(ctx context.Context, z22se bool) error {
	payload := []byte{0x06, REQUEST_DOWNLOAD, 0x00, 0x00, 0x00, 0x00, 0x00}
	if z22se {
		payload[0] = 0x05
	}
	frame := &gocan.CANFrame{
		Identifier: cl.canID,
		Data:       payload,
		FrameType:  gocan.ResponseRequired,
	}

	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("RequestDownload[1]: %w", err)
	}

	if err := CheckErr(resp); err != nil {
		return fmt.Errorf("RequestDownload[2]: %w", err)
	}

	if resp.Data[0] != 0x01 || resp.Data[1] != 0x74 {
		return errors.New("RequestDownload[3]: invalid response to RequestDownload")
	}

	return nil
}

// 8.13 TransferData ($36) Service.
//
// This service is used to transfer and/or execute a block of data, usually for
// reprogramming purposes.

// 80 downloadAndExecuteOrExecute
// This sub-parameter level of operation is used to command a node to receive a block
// transfer, download the data received to the memory address specified in the
// startingAddress[] parameter, and execute the data (program) downloaded. This subparameter command can also be used to execute a previously downloaded program by
// sending the request message with no data in the dataRecord[ ].

func (cl *Client) Execute(ctx context.Context, startAddress uint32) error {
	payload := []byte{
		0x06, TRANSFER_DATA, 0x80,
		byte(startAddress >> 24),
		byte(startAddress >> 16),
		byte(startAddress >> 8),
		byte(startAddress),
	}
	ti := time.Now()
	resp, err := cl.c.SendAndWait(ctx, gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired), cl.defaultTimeout*10, cl.recvID...)
	if err != nil {
		return fmt.Errorf("Execute[1]: %w", err)
	}
	log.Println("Execute took", time.Since(ti).Milliseconds())
	return CheckErr(resp)
}

// 00 Download
// This sub-parameter level of operation is used to command a node to receive a block
// transfer and (only) download the data received to the memory address specified in the
// startingAddress[] parameter.
func (cl *Client) TransferData(ctx context.Context, subFunc byte, length byte, startAddress int) error {
	payload := []byte{
		0x10, length, TRANSFER_DATA,
		subFunc,                  // Byte 3 is present when the memoryAddress parameter contains 3 or 4 bytes
		byte(startAddress >> 24), // Byte 4 is present , the memoryAddress parameter contains 4 bytes.
		byte(startAddress >> 16),
		byte(startAddress >> 8),
		byte(startAddress),
	}

	f := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndWait(ctx, f, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("TransferData[1]: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		return fmt.Errorf("TransferData[2]: %w", err)
	}

	if resp.Data[0] != 0x30 || resp.Data[1] != 0x00 {
		return errors.New("TransferData[3]: invalid response")
	}
	return nil
}

// 8.14 WriteDataByIdentifier ($3B) Service.
//
// The purpose of this service is to provide the ability to change
// write/program) the content of pre-defined ECU data referenced by a dataIdentifier (DID) which contains static
// information like ECU identification data, or other information which does not require real-time updates.

func (cl *Client) WriteDataByIdentifierUint16(ctx context.Context, pid byte, value uint16) error {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, value)
	return cl.WriteDataByIdentifier(ctx, pid, b)
}

func (cl *Client) WriteDataByIdentifierUint32(ctx context.Context, pid byte, value uint32) error {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(value))
	return cl.WriteDataByIdentifier(ctx, pid, b)
}

func (cl *Client) WriteDataByIdentifier(ctx context.Context, pid byte, data []byte) error {
	if len(data) > 6 {
		return cl.writeDataByIdentifierMultiframe(ctx, pid, data)
	}

	payload := []byte{byte(len(data) + 2), WRITE_DATA_BY_IDENTIFIER, pid}
	payload = append(payload, data...)
	//for i := len(payload); i < 8; i++ {
	//	payload = append(payload, 0x00)
	//}
	frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("WriteDataByIdentifier: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		//		log.Println(frame.String())
		//		log.Println(resp.String())
		return err
	}

	return nil
}

// $15 - WDBA - Write Data By Address
// This service is used to write data to a memory location in the ECU. All memory locations start with $15 so this is just a apptool call path inside the WriteDataBIdentifier
func (cl *Client) WriteDataByAddress(ctx context.Context, address uint32, data []byte) error {
	const (
		appToolPath         = 0x15
		firstFramePCI       = 0x10
		consecutiveFramePCI = 0x20
		flowControlPCI      = 0x30
		requiredBlockSize   = 0x01
		chunkSize           = 7 // 7 data bytes per consecutive frame
	)

	sendAndCheck := func(frame *gocan.CANFrame, where string) (*gocan.CANFrame, error) {
		resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		if err := CheckErr(resp); err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		return resp, nil
	}

	// ---- First frame --------------------------------------------------------
	first := &gocan.CANFrame{
		Identifier: cl.canID,
		Data: []byte{
			firstFramePCI, byte(len(data)) + 6, WRITE_DATA_BY_IDENTIFIER, appToolPath,
			byte(address >> 16), byte(address >> 8), byte(address), byte(len(data)),
		},
		FrameType: gocan.ResponseRequired,
	}
	resp, err := sendAndCheck(first, "WriteDataByAddress[1]")
	if err != nil {
		return err
	}
	if resp.Data[0] != flowControlPCI || resp.Data[1] != requiredBlockSize {
		return errors.New("WriteDataByAddress[3]: invalid response to initial request")
	}

	// ---- Consecutive frames -------------------------------------------------
	seq := byte(0x21)
	for off := 0; off < len(data); {
		n := min(len(data)-off, chunkSize)
		pkt := make([]byte, 0, 1+n)
		pkt = append(pkt, seq)
		pkt = append(pkt, data[off:off+n]...)
		off += n

		frame := &gocan.CANFrame{
			Identifier: cl.canID,
			Data:       pkt,
			FrameType:  gocan.ResponseRequired,
		}
		if _, err := sendAndCheck(frame, "WriteDataByAddress[4]"); err != nil {
			return err
		}

		seq = consecutiveFramePCI | ((seq + 1) & 0x0F)
	}

	return nil
}

func (cl *Client) WriteDataByAddress2(ctx context.Context, address uint32, data []byte) error {
	writeRequest := &gocan.CANFrame{
		Identifier: cl.canID,
		Data:       []byte{0x10, byte(len(data)) + 6, WRITE_DATA_BY_IDENTIFIER, 0x15, byte(address >> 16), byte(address >> 8), byte(address), byte(len(data))},
		FrameType:  gocan.ResponseRequired,
	}
	resp, err := cl.c.SendAndWait(ctx, writeRequest, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("WriteDataByAddress[1]: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		return fmt.Errorf("WriteDataByAddress[2]: %w", err)
	}
	if resp.Data[0] != 0x30 || resp.Data[1] != 0x01 {
		return errors.New("WriteDataByAddress[3]: invalid response to initial request")
	}
	seq := byte(0x21)
	for offset := 0; offset < len(data); {
		n := 7
		remaining := len(data) - offset
		if remaining < n {
			n = remaining
		}
		pkg := make([]byte, 1+n)
		pkg[0] = seq
		copy(pkg[1:], data[offset:offset+n])
		offset += n
		dataFrame := &gocan.CANFrame{
			Identifier: cl.canID,
			Data:       pkg,
			FrameType:  gocan.ResponseRequired,
		}
		resp, err := cl.c.SendAndWait(ctx, dataFrame, cl.defaultTimeout, cl.recvID...)
		if err != nil {
			return fmt.Errorf("WriteDataByAddress[4]: %w", err)
		}
		if err := CheckErr(resp); err != nil {
			return fmt.Errorf("WriteDataByAddress[5]: %w", err)
		}
		seq = 0x20 | ((seq + 1) & 0x0F)
	}
	return nil
}

func (cl *Client) writeDataByIdentifierMultiframe(ctx context.Context, pid byte, data []byte) error {
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
	leng := byte(len(data)) + 2
	payload := append([]byte{0x10, leng, WRITE_DATA_BY_IDENTIFIER, pid}, firstPart...)
	frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	//	log.Println(frame.String())
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return err
	}
	//log.Println(resp.String())
	if err := CheckErr(resp); err != nil {
		return err
	}

	if resp.Data[0] != 0x30 || resp.Data[1] > 0x01 {
		log.Println(resp.String())
		return errors.New("invalid response to initial writeDataByIdentifier")
	}

	delay := resp.Data[2]

	var seq byte = 0x21
	for r.Len() > 0 {
		pkg := []byte{seq}
	inner:
		for i := 1; i < 8; i++ {
			b, err := r.ReadByte()
			if err != nil {
				if err == io.EOF {
					//pkg = append(pkg, 0x00)
					break inner
				}
				return err
			}
			pkg = append(pkg, b)
		}

		if r.Len() > 0 {
			//log.Println(frame.String())
			cl.c.Send(cl.canID, pkg, gocan.Outgoing)
			time.Sleep(time.Duration(delay) * time.Millisecond)
		} else {
			frame := gocan.NewFrame(cl.canID, pkg, gocan.ResponseRequired)
			// log.Println(frame.String())
			resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
			if err != nil {
				return err
			}
			// log.Println(resp.String())
			if err := CheckErr(resp); err != nil {
				return err
			}
		}

		seq++
		if seq == 0x30 {
			seq = 0x20
		}
	}
	return nil
}

// 8.15 TesterPresent ($3E) Service
/*
 This service is used to indicate to a node (or nodes) that a tester is still
 connected to the vehicle and that certain diagnostic services that have been previously activated are to remain
 active. Some diagnostic services require that a tester send a request for this service periodically in order to
 keep the functionality of the other service active. Documentation within this specification of each diagnostic
 service indicates if a given service requires the periodic TesterPresent request to remain active
*/
func (cl *Client) TesterPresentResponseRequired(ctx context.Context) error {
	payload := []byte{0x01, 0x3E}
	f := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndWait(ctx, f, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("TesterPresentResponseRequired: %v", err)
	}
	return CheckErr(resp)
}

func (cl *Client) TesterPresentNoResponseAllowed() error {
	return cl.c.Send(0x101, []byte{0xFE, 0x01, 0x3E}, gocan.Outgoing)
}

// ReadDiagnosticInformation $A9 Service
//
//	readStatusOfDTCByStatusMask $81 Request
//	    DTCStatusMask $12= 0001 0010
//	      0 Bit 7 warningIndicatorRequestedState
//	      0 Bit 6 currentDTCSincePowerUp
//	      0 Bit 5 testNotPassedSinceCurrentPowerUp
//	      1 Bit 4 historyDTC
//	      0 Bit 3 testFailedSinceDTCCleared
//	      0 Bit 2 testNotPassedSinceDTCCleared
//	      1 Bit 1 currentDTC
//	      0 Bit 0 DTCSupportedByCalibration
func (cl *Client) ReadDiagnosticInformationStatusOfDTCByStatusMask(ctx context.Context, DTCStatusMask byte) ([][]byte, error) {
	return cl.readDiagnosticInformation(ctx, 0x81, []byte{DTCStatusMask})
}

// 8.16 ReportProgrammedState ($A2) Service.
//
// The reportProgrammedState is used by the tester to determine:
// * Which nodes on the link are programmable.
// * The current programmed state of each programmable node.
func (cl *Client) ReportProgrammedState(ctx context.Context) (byte, error) {
	frame := gocan.NewFrame(cl.canID, []byte{0x01, REPORT_PROGRAMMED_STATE}, gocan.ResponseRequired)
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return 0, fmt.Errorf("ReportProgrammedState[1]: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		return 0, fmt.Errorf("ReportProgrammedState[2]: %w", err)
	}
	if resp.Data[0] != 0x02 || resp.Data[1] != 0xE2 {
		return 0, errors.New("ReportProgrammedState[3]: invalid response")
	}
	return resp.Data[2], nil
}

func TranslateProgrammedState(state byte) string {
	switch state {
	case 0x00:
		return "fully programmed"
	case 0x01:
		return "no op s/w or cal data"
	case 0x02:
		return "op s/w present, cal data missing"
	case 0x03:
		return "s/w present, default or no start cal present"
	case 0x50:
		return "General Memory Fault"
	case 0x51:
		return "RAM Memory Fault"
	case 0x52:
		return "NVRAM Memory Fault"
	case 0x53:
		return "Boot Memory Failure"
	case 0x54:
		return "Flash Memory Failure"
	case 0x55:
		return "EEPROM Memory Failure "
	default:
		return "unknown"
	}
}

//8.17 ProgrammingMode ($A5) Service.
/*
This service provides for the following levels of operation:
* Verifies that all criteria are met to enable the programming services for all receiving nodes.
* Enables the high speed mode of operation (83.33 kbps) for all receiving nodes on the Single Wire CAN
(SWCAN) bus (if high speed programming was requested by the tool).
* Enables programming services for all receiving nodes.
This service shall only be available if normal communications have already been disabled (via service $28)

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
func (cl *Client) ProgrammingModeRequest(ctx context.Context) error {
	return cl.ProgrammingMode(ctx, 0x01)
}

func (cl *Client) ProgrammingModeRequestHighSpeed(ctx context.Context) error {
	return cl.ProgrammingMode(ctx, 0x02)
}

func (cl *Client) ProgrammingModeEnable(ctx context.Context) error {
	return cl.ProgrammingMode(ctx, 0x03)
}

func (cl *Client) ProgrammingMode(ctx context.Context, subFunc byte) error {
	payload := []byte{0x02, PROGRAMMING_MODE, subFunc}
	switch subFunc {
	case 0x01, 0x02:
		frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
		resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout*4, cl.recvID...)
		if err != nil {
			return fmt.Errorf("ProgrammingMode[1]: %X %w", subFunc, err)
		}
		if resp.Data[0] != 0x01 || resp.Data[1] != 0xE5 {
			return errors.New("ProgrammingMode[2]: invalid response")
		}
		return nil

	case 0x03:
		if err := cl.c.Send(cl.canID, payload, gocan.Outgoing); err != nil {
			return err
		}
		return nil
	default:
		return errors.New("ProgrammingMode[3]: unknown subFunc")
	}
}

// 8.18 ReadDiagnosticInformation ($) Service.
//
// This service allows a tester to read the status of
// node-resident Diagnostic Trouble Code (DTC) information from any controller, or group of controllers within a
// vehicle. This service allows the tester to do the following:
// 1. Retrieve the status of a specific DTC and FaultType combination.
// 2. Retrieve the list of DTCs that match a tester defined DTC status mask.
// 3. Enable a node resident algorithm which periodically calculates the number of DTCs that match a tester
// defined DTC status mask. The ECU shall send a response message each time the calculation yields a
// different result than the one calculated the previous time.
func (cl *Client) readDiagnosticInformation(ctx context.Context, subFunc byte, payload []byte) ([][]byte, error) {
	if len(payload) > 3 {
		return nil, errors.New("to big payload for readDiagnosticInformation")
	}
	header := []byte{0x03, 0xA9, subFunc}
	frame := gocan.NewFrame(cl.canID, append(header, payload...), gocan.CANFrameType{Type: 2, Responses: 15})
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return nil, err
	}

	var out [][]byte
	if err := CheckErr(resp); err != nil {
		if strings.Contains(err.Error(), "Response pending") {
			ch := cl.c.Subscribe(ctx, cl.recvID...)
			defer ch.Close()
		outer:
			for {
				select {
				case resp := <-ch.Chan():
					if resp.Data[1] == 0x00 && resp.Data[2] == 0x00 && resp.Data[3] == 0x00 { // No more DTCs
						break outer
					}
					//log.Println("append")
					out = append(out, []byte{resp.Data[1], resp.Data[2], resp.Data[3], resp.Data[4]})
				case <-time.After(00 * time.Millisecond):
					break outer
				}
			}
		} else {
			//log.Println("not pending")
			return nil, err
		}
	}
	return out, nil
}

// 8.19 ReadDataByPacketIdentifier ($AA) Service.
/*
The purpose of the ReadDataByPacketIdentifier($AA)
service is to allow a tester to request data packets that contain diagnostic information (e.g., sensor input or
output values) which are packaged in a UUDT diagnostic message format. Refer to paragraph 4.5.1.2 for more
information on UUDT diagnostic message format. Each diagnostic data packet includes one byte that contains
a Data Packet IDentifier (DPID) number, and one to seven bytes of additional data. The DPID number
occupies the message number byte position of the UUDT diagnostic response message and is used by the
tester to determine the data contents of the remaining bytes of the message.
This service is intended to be used to retrieve ECU data which is most likely changing during normal operation
(e.g., ECU sensor inputs, ECU commanded output states, etc). Static information such as VIN or Part
Numbers should be retrieved via the ReadDataByIdentifier ($1A) service.
The DPIDs requested via this service can be sent as a one-time response or scheduled periodically. Each
DPID scheduled can be transmitted at one of three predefined periodic rates (slow, medium, or fast). Periodic
rates require a TesterPresent ($3E) message to be sent on the bus to keep the Periodic DPID Scheduler
(PDS) active (reference $3E service description).
*/
func (cl *Client) ReadDataByPacketIdentifier(ctx context.Context, subFunc byte, dpid ...byte) ([]byte, error) {
	f := gocan.NewFrame(
		cl.canID,
		append(
			[]byte{byte(len(dpid) + 2), READ_DATA_BY_PACKET_IDENTIFIER, subFunc},
			dpid...,
		),
		gocan.CANFrameType{
			Type:      2,
			Responses: len(dpid),
		},
	)
	//	log.Println(f.String())

	resp, err := cl.c.SendAndWait(ctx, f, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return nil, fmt.Errorf("ReadDataByPacketIdentifier: %w", err)
	}

	// log.Println(resp.String())

	if err := CheckErr(resp); err != nil {
		return nil, err
	}

	/*
		d := resp.Data()

		log.Printf("messages %d", d[2])

		noResp := d[2]

		for noResp > 0 {
			dd, err := cl.c.Poll(ctx, cl.defaultTimeout, cl.recvID...)
			if err != nil {
				return nil, fmt.Errorf("ReadDataByPacketIdentifier: %w", err)
			}
			log.Println(dd.String())
			noResp--
		}
	*/
	return resp.Data, nil
}

//8.20 DeviceControl ($AE) Service.
//The purpose of this service is to allow a test device to override normal
//output control functions in order to verify proper operation of a component or system, or to reset/clear variables
//used within normal control algorithms. The tool may take control of multiple outputs simultaneously with a
//single request message or by sending multiple device control service messages.

func (cl *Client) DeviceControl(ctx context.Context, command byte) error {
	payload := []byte{0x02, DEVICE_CONTROL, command}
	frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return err
	}
	return CheckErr(resp)
}

func (cl *Client) DeviceControlWithCode(ctx context.Context, command byte, code []byte) error {
	payload := []byte{0x07, DEVICE_CONTROL, command}
	payload = append(payload, code...)
	frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndWait(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return err
	}
	return CheckErr(resp)
}

/*
func (cl *Client) ReadDataByIdentifier(ctx context.Context, pid byte) ([]byte, error) {
	out := bytes.NewBuffer([]byte{})
	f := gocan.NewFrame(cl.canID, []byte{0x02, 0x1A, pid}, gocan.ResponseRequired)
	resp, err := cl.c.SendAndWait(ctx, f, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return nil, err
	}
	if err := CheckErr(resp); err != nil {
		return nil, err
	}
	d := resp.Data()
	switch {
	case d[1] == 0x5A: // only one frame in this response
		length := d[0]
		for i := 3; i <= int(length); i++ {
			out.WriteByte(d[i])
		}
		return out.Bytes(), nil
	case d[2] == 0x5A:
		leng := d[1]

		lenThisFrame := int(leng)
		if lenThisFrame > 4 {
			lenThisFrame = 4
		}

		left := int(leng) - 2
		framesToReceive := (leng - 4) / 8
		if (leng-4)%8 > 0 {
			framesToReceive++
		}

		for i := 4; i < 4+lenThisFrame; i++ {
			out.WriteByte(d[i])
			left--
		}

		cl.c.SendFrame(cl.canID, []byte{0x30, 0x00, 0x00}, gocan.CANFrameType{Type: 2, Responses: int(framesToReceive)})

		var seq byte = 0x21

		cc := cl.c.Subscribe(ctx, cl.recvID...)

		for framesToReceive > 0 {
			select {
			case resp2 := <-cc:
				//resp2, err := cl.c.Poll(ctx, cl.defaultTimeout, cl.recvID...)
				//if err != nil {
				//	return nil, err
				//}
				if err := CheckErr(resp2); err != nil {
					return nil, err
				}
				frameData := resp2.Data()
				if frameData[0] != seq {
					return nil, fmt.Errorf("frame sequence out of order, expected 0x%X got 0x%X", seq, frameData[0])
				}

				for i := 1; i < 8; i++ {
					out.WriteByte(frameData[i])
					left--
					if left == 0 {
						return out.Bytes(), nil
					}
				}
				framesToReceive--
				seq++
				if seq == 0x30 {
					seq = 0x20
				}
			case <-time.After(5 * time.Second):
				return nil, errors.New("timeout waiting for response")
			}
		}
		return out.Bytes(), nil
	default:
		//log.Println(resp.String())
		return nil, errors.New("unhandled response")
	}
	//log.Println(resp.String())
}
*/
