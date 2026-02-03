package main

import (
	"fmt"
	"log"
	"time"
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

	if err := pt.PassThruConnect(deviceID, passthru.ISO14230, passthru.ISO9141_NO_CHECKSUM | passthru.ISO9141_K_LINE_ONLY, 10400, &channelID); err != nil {
		return fmt.Errorf("PassThruConnect: %w", err)
	}
	defer pt.PassThruDisconnect(channelID)

	opts := &passthru.SCONFIG_LIST{
		NumOfParams: 3,
		Params: []passthru.SCONFIG{
			{
				Parameter: passthru.P4_MIN,
				Value:     0xA,
			},
			{
				Parameter: passthru.TIDLE,
				Value:     0x1,
			},
			{
				Parameter: passthru.TWUP,
				Value:     0x32,
			},
			{
				Parameter: passthru.TINIL,
				Value:     0x19,
			},
		},
	}
	if err := pt.PassThruIoctl(channelID, passthru.SET_CONFIG, opts); err != nil {
		return fmt.Errorf("PassThruIoctl set options: %w", err)
	}

	opts = &passthru.SCONFIG_LIST{
		NumOfParams: 2,
		Params: []passthru.SCONFIG{
			{
				Parameter: passthru.P3_MIN,
				Value:     110,
			},
			{
				Parameter: passthru.LOOPBACK,
				Value:     0,
			},
		},
	}
	if err := pt.PassThruIoctl(channelID, passthru.SET_CONFIG, opts); err != nil {
		return fmt.Errorf("PassThruIoctl set options: %w", err)
	}

	filterID := uint32(0)

	mask := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
		DataSize:   4,
		Data:       [4128]byte{0x00, 0x00, 0x00, 0x00},
	}

	pattern := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
		DataSize:   4,
		Data:       [4128]byte{0x00, 0x00, 0x00, 0x00},
	}

	if err := pt.PassThruStartMsgFilter(channelID, passthru.PASS_FILTER, mask, pattern, nil, &filterID); err != nil {
		return fmt.Errorf("PassThruStartMsgFilter: %w", err)
	}

	txMSG := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
		DataSize:   5,
		Data: [4128]byte{0x81, 0x41, 0xF1, 0x81, 0x34}, // 81 41 F1 81
	}

	rxMSG := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
	}

	if err := pt.PassThruIoctl(channelID, passthru.FAST_INIT, txMSG, rxMSG); err != nil {
		return fmt.Errorf("PassThruIoctl fast init: %w", err)
	}

	msg1 := uint32(1)
	noMessages := uint32(2)
	rxMSGS := &passthru.PassThruMsg{}

	txMSGT := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
		DataSize:   6,
		Data: [4128]byte{0x80, 0x41, 0xF1, 0x01, 0x3E, 0xF1},
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

	txMSG2 := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
		DataSize:   7,
		Data: [4128]byte{0x80, 0x41, 0xF1, 0x02, 0x21, 0x24, 0xF9},
	}
	if err := pt.PassThruWriteMsgs(channelID, txMSG2, &msg1, 500); err != nil {
		return fmt.Errorf("PassThruWriteMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &noMessages, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}

	if err := pt.PassThruWriteMsgs(channelID, txMSG2, &msg1, 500); err != nil {
		return fmt.Errorf("PassThruWriteMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &noMessages, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}

	log.Println("lets waits 10s, check if adapter tester present is working, if crash then its not, otherwise read VIN") 
	time.Sleep(10 * time.Second)
	txMSG3 := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
		DataSize:   7,
		Data: [4128]byte{0x80, 0x41, 0xF1, 0x02, 0x1A, 0x80, 0x4E},
	}
	if err := pt.PassThruWriteMsgs(channelID, txMSG3, &msg1, 500); err != nil {
		return fmt.Errorf("PassThruWriteMsgs: %w", err)
	}

	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &msg1, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &msg1, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}
	log.Println("VIN: " + string(rxMSGS.Data[5:22]))

	txMSG4 := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
		DataSize:   7,
		Data: [4128]byte{0x80, 0x41, 0xF1, 0x02, 0x1A, 0x80, 0x4A}, //bad checksum
	}
	log.Println("Wrong CRC in msg, shall fail: ")
	if err := pt.PassThruWriteMsgs(channelID, txMSG4, &msg1, 500); err != nil {
		log.Println(fmt.Errorf("PassThruWriteMsgs: %w", err))
	}

	return nil
}
