package t8

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/gocan/pkg/model"
)

var T8Headers = []model.Header{
	{Desc: "VIN", ID: 0x90, Type: "string"},
	{Desc: "Calibration set ", ID: 0x74, Type: "string"},
	{Desc: "Codefile version", ID: 0x73, Type: "string"},
	{Desc: "ECU description", ID: 0x72, Type: "string"},
	{Desc: "ECU hardware", ID: 0x71, Type: "string"},
	{Desc: "ECU s/w number", ID: 0x95, Type: "hex"},
	{Desc: "Programming date", ID: 0x99, Type: "hex"},
	{Desc: "Build date", ID: 0x0A, Type: "string"},
	{Desc: "Serial number", ID: 0xB4, Type: "string"},
	{Desc: "Software version", ID: 0x08, Type: "string"},
	{Desc: "0F identifier   ", ID: 0x0F, Type: "string"},
	{Desc: "SW identifier 1", ID: 0xC1, Type: "string"},
	{Desc: "SW identifier 2", ID: 0xC2, Type: "string"},
	{Desc: "SW identifier 3", ID: 0xC3, Type: "string"},
	{Desc: "SW identifier 4", ID: 0xC4, Type: "string"},
	{Desc: "SW identifier 5", ID: 0xC5, Type: "string"},
	{Desc: "SW identifier 6", ID: 0xC6, Type: "string"},
	{Desc: "Hardware type", ID: 0x97, Type: "string"},
	{Desc: "75 identifier", ID: 0x75, Type: "string"},
	{Desc: "Engine type", ID: 0x0C, Type: "string"},
	{Desc: "Supplier ID", ID: 0x92, Type: "string"},
	{Desc: "Speed limiter", ID: 0x02, Type: "km/h"},
	{Desc: "Oil quality", ID: 0x25, Type: "oilquality"},
	{Desc: "SAAB partnumber", ID: 0x7C, Type: "int64"},
	{Desc: "Diagnostic Data Identifier", ID: 0x9A, Type: "ddi"},
	{Desc: "End model partnumber", ID: 0xCB, Type: "int64"},
	{Desc: "Base model partnumber", ID: 0xCC, Type: "int64"},
	{Desc: "ManufacturersEnableCounter", ID: 0xA0, Type: "uint32"},
	{Desc: "Tester Serial", ID: 0x98, Type: "string"},
	{Desc: "E85", ID: 0x7A, Type: "e85"},
}

func (t *Client) Info(ctx context.Context, callback model.ProgressCallback) ([]model.HeaderResult, error) {
	gm := gmlan.New(t.c)
	if callback != nil {
		callback(-float64(len(T8Headers)))
		callback("Initialize session")
	}
	gm.TesterPresentNoResponseAllowed()

	time.Sleep(20 * time.Millisecond)

	gm.DisableNormalCommunicationAllNodes()
	if err := gm.DisableNormalCommunication(ctx, 0x7E0, 0x7E8); err != nil {
		return nil, err
	}

	n := 0
	//var out []model.HeaderResult
	for i, h := range T8Headers {
		n++
		if n == 5 {
			gm.TesterPresentNoResponseAllowed()
			n = 0
		}
		switch h.Type {
		case "string":
			data, err := t.RequestECUInfoAsString(ctx, h.ID)
			if err != nil {
				return nil, err
			}
			if callback != nil {
				callback(fmt.Sprintf("%-27s: %s", h.Desc, data))
			}
		case "int64":
			data, err := t.RequestECUInfoAsInt64(ctx, h.ID)
			if err != nil {
				return nil, err
			}
			if callback != nil {
				callback(fmt.Sprintf("%-27s: %d", h.Desc, data))
			}
		case "hex":
			data, err := t.RequestECUInfo(ctx, h.ID)
			if err != nil {
				return nil, err
			}
			if callback != nil {
				callback(fmt.Sprintf("%-27s: %X", h.Desc, data))
			}
		case "uint32":
			data, err := t.RequestECUInfoAsUint32(ctx, h.ID)
			if err != nil {
				return nil, err
			}
			if callback != nil {
				callback(fmt.Sprintf("%-27s: %d", h.Desc, data))
			}
		case "km/h":
			data, err := t.RequestECUInfo(ctx, h.ID)
			if err != nil {
				return nil, err
			}
			var retval uint32
			if len(data) == 2 {
				retval = uint32(data[0]) * 256
				retval += uint32(data[1])
				retval /= 10
			}
			if callback != nil {
				callback(fmt.Sprintf("%-27s: %d km/h", h.Desc, retval))
			}
		case "oilquality":
			data, err := t.RequestECUInfoAsUint64(ctx, h.ID)
			if err != nil {
				return nil, err
			}
			quality := float64(data) / 256

			if callback != nil {
				callback(fmt.Sprintf("%-27s: %.2f %%", h.Desc, quality))
			}
		case "ddi":
			data, err := t.RequestECUInfo(ctx, h.ID)
			if err != nil {
				return nil, err
			}
			var retval string
			if len(data) == 2 {
				retval = fmt.Sprintf("0x%02X 0x%02X", data[0], data[1])
			}
			if callback != nil {
				callback(fmt.Sprintf("%-27s: %s", h.Desc, retval))
			}
		case "e85":
			gm := gmlan.New(t.c)
			data, err := gm.ReadDataByPacketIdentifier(ctx, 0x7E0, 0x7E8, 0x01, 0x7A)
			if err != nil && err.Error() != "Request out of range or session dropped" {
				return nil, err
			}
			if len(data) == 2 {
				e85 := uint32(data[2])
				if callback != nil {
					callback(fmt.Sprintf("%-27s: %d %%", h.Desc, e85))
				}
			} else {
				if callback != nil {
					callback(fmt.Sprintf("%-27s: %d %%", "E85", 0))
				}
			}

		default:

		}
		if callback != nil {
			callback(float64(i + 1))
		}
		//a := model.HeaderResult{Value: resp}
		//a.Desc = h.Desc
		//a.ID = h.ID
		//out = append(out, a)
	}

	return nil, nil
}
func (t *Client) RequestECUInfoAsString(ctx context.Context, pid byte) (string, error) {
	resp, err := t.RequestECUInfo(ctx, pid)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(resp[:]), "\x00", ""), nil
}

func (t *Client) RequestECUInfoAsUint32(ctx context.Context, pid byte) (uint32, error) {
	resp, err := t.RequestECUInfo(ctx, pid)
	if err != nil {
		return 0, err
	}
	var retval uint32
	binary.Read(bytes.NewReader(resp), binary.BigEndian, &retval)
	return retval, nil
}

func (t *Client) RequestECUInfoAsInt64(ctx context.Context, pid byte) (int64, error) {
	resp, err := t.RequestECUInfo(ctx, pid)
	if err != nil {
		return 0, err
	}
	var retval int64
	retval += int64(resp[0]) * 256 * 256 * 256
	retval += int64(resp[1]) * 256 * 256
	retval += int64(resp[2]) * 256
	retval += int64(resp[3])
	return retval, nil
}

func (t *Client) RequestECUInfoAsUint64(ctx context.Context, pid byte) (uint64, error) {
	resp, err := t.RequestECUInfo(ctx, pid)
	if err != nil {
		return 0, err
	}
	var retval uint64
	retval += uint64(resp[0]) * 256 * 256 * 256
	retval += uint64(resp[1]) * 256 * 256
	retval += uint64(resp[2]) * 256
	retval += uint64(resp[3])
	return retval, nil
}

func (t *Client) RequestECUInfo(ctx context.Context, pid byte) ([]byte, error) {
	gm := gmlan.New(t.c)
	return gm.ReadDataByIdentifier(ctx, 0x7E0, 0x7E8, pid)
}

func (t *Client) SendAckMessageT8() {
	if err := t.c.SendFrame(0x7E0, []byte{0x30}, gocan.Outgoing); err != nil {
		panic(err)
	}
}
