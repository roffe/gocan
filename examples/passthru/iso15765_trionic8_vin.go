package main

import (
	"fmt"
	"log"
	"github.com/roffe/gocan/pkg/passthru"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
}

func main() {
	if err := asd(); err != nil {
		log.Fatal(err)
	}
}

func asd() error {
	pt, err := passthru.New(`/home/witold/.passthru/libj2534drv.so`)
	if err != nil {
		log.Fatal(err)
	}
	var deviceID uint32 = 1
	var channelID uint32 = 1
	if err := pt.PassThruOpen("", &deviceID); err != nil {
		return fmt.Errorf("PassThruOpen: %w", err)
	}
	defer pt.PassThruClose(deviceID)

	if err := pt.PassThruConnect(deviceID, passthru.ISO15765, 0, 500000, &channelID); err != nil {
		return fmt.Errorf("PassThruConnect: %w", err)
	}
	defer pt.PassThruDisconnect(channelID)

	filterID := uint32(0)

	mask := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO15765,
		DataSize:   4,
		Data:       [4128]byte{0xFF, 0xFF, 0xFF, 0xFF},
	}

	pattern := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO15765,
		DataSize:   4,
		Data:       [4128]byte{0x00, 0x00, 0x07, 0xE8},
	}

	flow := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO15765,
		DataSize:   4,
		Data:       [4128]byte{0x00, 0x00, 0x07, 0xE0},
	}

	if err := pt.PassThruStartMsgFilter(channelID, passthru.FLOW_CONTROL_FILTER, mask, pattern, flow, &filterID); err != nil {
		return fmt.Errorf("PassThruStartMsgFilter: %w", err)
	}

	msg1 := uint32(1)
	rxMSGS := &passthru.PassThruMsg{}
	txMSGT := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO15765,
		DataSize:   6,
		Data: [4128]byte{0x00, 0X00, 0x07, 0xDF, 0x09, 0x02},
	}
	if err := pt.PassThruWriteMsgs(channelID, txMSGT, &msg1, 500); err != nil {
		return fmt.Errorf("PassThruWriteMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &msg1, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &msg1, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &msg1, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}
	log.Println("VIN: " + string(rxMSGS.Data[7:24]))

	txMSGT = &passthru.PassThruMsg{
		ProtocolID: passthru.ISO15765,
		DataSize:   6,
		Data: [4128]byte{0x00, 0X00, 0x07, 0xDF, 0x09, 0x04},
	}
	if err := pt.PassThruWriteMsgs(channelID, txMSGT, &msg1, 500); err != nil {
		return fmt.Errorf("PassThruWriteMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &msg1, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &msg1, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &msg1, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err) 
	}
	log.Println("Calibration: " + string(rxMSGS.Data[7:23]))

	return nil
}
