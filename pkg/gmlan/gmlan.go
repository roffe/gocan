// see GMW3110 gmw3110-2010.pdf
package gmlan

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/roffe/gocan"
)

type Client struct {
	c              *gocan.Client
	defaultTimeout time.Duration
	canID          uint32
	recvID         []uint32
}

func New(c *gocan.Client, canID uint32, recvID ...uint32) *Client {
	return &Client{
		c:              c,
		defaultTimeout: 150 * time.Millisecond,
		canID:          canID,
		recvID:         recvID,
	}
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
		0x06, 0x36, 0x80,
		byte(startAddress >> 24),
		byte(startAddress >> 16),
		byte(startAddress >> 8),
		byte(startAddress),
	}
	resp, err := cl.c.SendAndPoll(ctx, gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired), cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return err
	}
	return CheckErr(resp)
}

//8.20 DeviceControl ($AE) Service.
//The purpose of this service is to allow a test device to override normal
//output control functions in order to verify proper operation of a component or system, or to reset/clear variables
//used within normal control algorithms. The tool may take control of multiple outputs simultaneously with a
//single request message or by sending multiple device control service messages.

func (cl *Client) DeviceControl(ctx context.Context, command byte) error {
	payload := []byte{0x02, 0xAE, command}
	frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return err
	}
	return CheckErr(resp)
}

func (cl *Client) DeviceControlWithCode(ctx context.Context, command byte, code []byte) error {
	payload := []byte{0x07, 0xAE, command}
	payload = append(payload, code...)
	frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return err
	}
	return CheckErr(resp)
}

// 00 Download
// This sub-parameter level of operation is used to command a node to receive a block
// transfer and (only) download the data received to the memory address specified in the
// startingAddress[] parameter.
func (cl *Client) TransferData(ctx context.Context, subFunc byte, length byte, startAddress int) error {
	payload := []byte{
		0x10, length, 0x36,
		subFunc,                  // Byte 3 is present when the memoryAddress parameter contains 3 or 4 bytes
		byte(startAddress >> 24), // Byte 4 is present , the memoryAddress parameter contains 4 bytes.
		byte(startAddress >> 16),
		byte(startAddress >> 8),
		byte(startAddress),
	}

	f := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, f, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return err
	}

	if err := CheckErr(resp); err != nil {
		return err
	}

	d := resp.Data()
	if d[0] != 0x30 || d[1] != 0x00 {
		return errors.New("/!\\ Did not receive correct response from TransferData")
	}

	return nil
}

//8.9 DisableNormalCommunication ($28) Service.
/*
The purpose of this service is to prevent a device from
transmitting or receiving all messages which are not the direct result of a diagnostic request. The primary use
of the service is to set up a programming event. This is a required service that must be supported by all nodes
*/
func (cl *Client) DisableNormalCommunication(ctx context.Context) error {
	frame := gocan.NewFrame(cl.canID, []byte{0x01, 0x28}, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("DisableNormalCommunication: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		return err
	}
	d := resp.Data()
	if d[0] != 0x01 || d[1] != 0x68 {
		return errors.New("invalid response to DisableNormalCommunication")
	}
	return nil
}

// AllNodes functional diagnostic request CANId ($101) and the AllNodes extended address ($FE).
func (cl *Client) DisableNormalCommunicationAllNodes() error {
	if err := cl.c.SendFrame(0x101, []byte{0xFE, 0x01, 0x28}, gocan.Outgoing); err != nil {
		return err
	}
	return nil
}

// 8.2 InitiateDiagnosticOperation ($10) Service.
/*
This service allows the tester to perform the following tasks:
* Disable the setting of all DTCs while the tool continues to perform other diagnostic services.
* Allow ECU DTC algorithms to continue to execute while the DeviceControl ($AE) service is active.
* Request a gateway ECU to issue a wake-up request.

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
func (cl *Client) InitiateDiagnosticOperation(ctx context.Context, subFunc byte) error {
	payload := []byte{0x02, 0x10, subFunc}
	frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("InitiateDiagnosticOperation: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		return err
	}
	d := resp.Data()
	if d[0] != 0x01 || d[1] != 0x50 {
		return errors.New("invalid response to InitiateDiagnosticOperation request")
	}

	return nil
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
	return cl.programmingMode(ctx, 0x01)
}

func (cl *Client) ProgrammingModeRequestHighSpeed(ctx context.Context) error {
	return cl.programmingMode(ctx, 0x02)
}

func (cl *Client) ProgrammingModeEnable(ctx context.Context) error {
	return cl.programmingMode(ctx, 0x03)
}

func (cl *Client) programmingMode(ctx context.Context, subFunc byte) error {
	payload := []byte{0x02, 0xA5, subFunc}
	switch subFunc {
	case 0x01, 0x02:
		frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
		resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
		if err != nil {
			return err
		}
		d := resp.Data()
		if d[0] != 0x01 || d[1] != 0xE5 {
			return errors.New("request ProgrammingMode invalid response")
		}
		return nil

	case 0x03:
		frame := gocan.NewFrame(cl.canID, payload, gocan.Outgoing)
		if err := cl.c.Send(frame); err != nil {
			return err
		}
		return nil
	default:
		return errors.New("ProgrammingMode: unknown subFunc")
	}
}

// $20
func (cl *Client) ReturnToNormalMode(ctx context.Context) error {
	resp, err := cl.c.SendAndPoll(
		ctx,
		gocan.NewFrame(cl.canID, []byte{0x01, 0x20}, gocan.ResponseRequired),
		cl.defaultTimeout,
		cl.recvID...,
	)
	if err != nil {
		return fmt.Errorf("ReturnToNormalMode: %w", err)
	}
	return CheckErr(resp)
}

// 8.16 ReportProgrammedState ($A2) Service.
//
// The reportProgrammedState is used by the tester to determine:
// * Which nodes on the link are programmable.
// * The current programmed state of each programmable node.
func (cl *Client) ReportProgrammedState(ctx context.Context) error {
	frame := gocan.NewFrame(cl.canID, []byte{0x01, 0xA2}, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("ReportProgrammedState: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		return err
	}
	d := resp.Data()
	if d[0] != 0x02 || d[1] != 0xE2 {
		return errors.New("invalid response to ReportProgrammedState request")
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
	out := bytes.NewBuffer([]byte{})
	f := gocan.NewFrame(cl.canID, []byte{0x02, 0x1A, pid}, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, f, cl.defaultTimeout, cl.recvID...)
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
			[]byte{byte(len(dpid) + 2), 0xAA, subFunc},
			dpid...,
		),
		gocan.CANFrameType{
			Type:      2,
			Responses: len(dpid),
		},
	)
	resp, err := cl.c.SendAndPoll(ctx, f, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return nil, fmt.Errorf("ReadDataByPacketIdentifier: %w", err)
	}

	if err := CheckErr(resp); err != nil {
		return nil, err
	}

	return resp.Data(), nil
}

//func (cl *Client) sendAndReceive(ctx context.Context, payload []byte) (gocan.CANFrame, error) {
//	frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
//	resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
//	if err != nil {
//		return nil, err
//	}
//	return resp, CheckErr(resp)
//}

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
	resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return nil, err
	}

	var out [][]byte
	if err := CheckErr(resp); err != nil {
		if strings.Contains(err.Error(), "Response pending") {
			ch := cl.c.Subscribe(ctx, cl.recvID...)
		outer:
			for {
				select {
				case resp := <-ch:
					d := resp.Data()
					if d[1] == 0x00 && d[2] == 0x00 && d[3] == 0x00 { // No more DTCs
						break outer
					}
					//log.Println("append")
					out = append(out, []byte{d[1], d[2], d[3], d[4]})
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

// 8.8 SecurityAccess ($27) Service.
//
// The purpose of this service is to provide a means to access data and/or
// diagnostic services which have restricted access for security, emissions, or safety reasons. Diagnostic modes
// for downloading routines or data into a node and reading specific memory locations from a node are situations
// where security access may be required. Improper routines or data downloaded into a node could potentially
// damage the electronics or other vehicle components or risk the vehicle’s compliance to emission, safety, or
// security standards. This mode is intended
func (cl *Client) SecurityAccessRequestSeed(ctx context.Context, accessLevel byte) ([]byte, error) {
	payload := []byte{0x02, 0x27, accessLevel}
	frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return nil, fmt.Errorf("SecurityAccessRequestSeed: %w", err)
	}
	if err := CheckErr(resp); err != nil {
		return nil, err
	}
	d := resp.Data()
	if d[1] != 0x67 || d[2] != accessLevel {
		return nil, errors.New("invalid Response to SecurityAccessRequestSeed")
	}

	return []byte{d[3], d[4]}, nil
}

func (cl *Client) SecurityAccessSendKey(ctx context.Context, accessLevel, high, low byte) error {
	respPayload := []byte{0x04, 0x27, accessLevel + 0x01, high, low}
	frame := gocan.NewFrame(cl.canID, respPayload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("SecurityAccessSendKey: %w", err)
	}

	if err := CheckErr(resp); err != nil {
		return err
	}

	d := resp.Data()
	if d[1] == 0x67 && d[2] == accessLevel+0x01 {
		//log.Println("Security access granted")
		return nil
	}
	return errors.New("/!\\ Failed to obtain security access")
}

func (cl *Client) RequestSecurityAccess(ctx context.Context, accesslevel byte, delay time.Duration, seedfunc func([]byte, byte) (byte, byte)) error {
	//time.Sleep(50 * time.Millisecond)
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

	if err := cl.SecurityAccessSendKey(ctx, accesslevel, high, low); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return nil
}

// 8.12 RequestDownload ($34) Service. This service is used in order to prepare a node to be programmed
func (cl *Client) RequestDownload(ctx context.Context, z22se bool) error {
	payload := []byte{0x06, 0x34, 0x00, 0x00, 0x00, 0x00, 0x00}

	if z22se {
		payload[0] = 0x05
	}

	f := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, f, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return err
	}

	if err := CheckErr(resp); err != nil {
		return err
	}

	d := resp.Data()
	if d[0] != 0x01 || d[1] != 0x74 {
		return errors.New("Did not receive correct response from RequestDownload") //lint:ignore ST1005 ignore this
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
	resp, err := cl.c.SendAndPoll(ctx, f, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return fmt.Errorf("TesterPresentResponseRequired: %v", err)
	}
	return CheckErr(resp)
}

func (cl *Client) TesterPresentNoResponseAllowed() error {
	return cl.c.Send(gocan.NewFrame(0x101, []byte{0xFE, 0x01, 0x3E}, gocan.Outgoing))
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
	if len(data) > 5 {
		return cl.writeDataByIdentifierMultiframe(ctx, pid, data)
	}

	payload := []byte{byte(len(data) + 2), 0x3B, pid}
	payload = append(payload, data...)
	//for i := len(payload); i < 8; i++ {
	//	payload = append(payload, 0x00)
	//}
	frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
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
	payload := []byte{0x10, leng, 0x3B, pid}
	payload = append(payload, firstPart...)
	frame := gocan.NewFrame(cl.canID, payload, gocan.ResponseRequired)
	//	log.Println(frame.String())
	resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
	if err != nil {
		return err
	}
	//log.Println(resp.String())
	d := resp.Data()

	if err := CheckErr(resp); err != nil {
		return err
	}

	if d[0] != 0x30 || d[1] != 0x00 {
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
					//pkg = append(pkg, 0x00)
					break inner
				}
				return err
			}
			pkg = append(pkg, b)
		}

		if r.Len() > 0 {
			frame := gocan.NewFrame(cl.canID, pkg, gocan.Outgoing)
			//log.Println(frame.String())
			cl.c.Send(frame)
			time.Sleep(time.Duration(delay) * time.Millisecond)
		} else {
			frame := gocan.NewFrame(cl.canID, pkg, gocan.ResponseRequired)
			// log.Println(frame.String())
			resp, err := cl.c.SendAndPoll(ctx, frame, cl.defaultTimeout, cl.recvID...)
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
