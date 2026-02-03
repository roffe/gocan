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

	if err := pt.PassThruConnect(deviceID, passthru.ISO14230, passthru.ISO9141_K_LINE_ONLY, 10400, &channelID); err != nil {
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
	msg1 := uint32(1)
	noMsg1 := uint32(1)
	noMsg2 := uint32(2)
	rxMSGS := &passthru.PassThruMsg{}

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
	//send msg before Fast init, it shall FAIL
	txMSG2 := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
		DataSize:   6,
		Data: [4128]byte{0x80, 0x41, 0xF1, 0x02, 0x21, 0x24},
	}
	log.Println("seng msg before FAST INIT prosedure, it shall fail:");
	if err := pt.PassThruWriteMsgs(channelID, txMSG2, &msg1, 500); err != nil {
		log.Println(err);
	}

	//fast init
	txMSG := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
		DataSize:   4,
		Data: [4128]byte{0x81, 0x41, 0xF1, 0x81}, // 81 41 F1 81
	}

	rxMSG := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
	}

	if err := pt.PassThruIoctl(channelID, passthru.FAST_INIT, txMSG, rxMSG); err != nil {
		return fmt.Errorf("PassThruIoctl fast init: %w", err)
	}
	if err := pt.PassThruWriteMsgs(channelID, txMSG2, &msg1, 500); err != nil {
		return fmt.Errorf("PassThruWriteMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &noMsg2, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}
	if err := pt.PassThruWriteMsgs(channelID, txMSG2, &msg1, 500); err != nil {
		return fmt.Errorf("PassThruWriteMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &noMsg2, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}

	txMSG3 := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
		DataSize:   6,
		Data: [4128]byte{0x80, 0x41, 0xF1, 0x02, 0x1a, 0x80},
	}
	if err := pt.PassThruWriteMsgs(channelID, txMSG3, &msg1, 500); err != nil {
		return fmt.Errorf("PassThruWriteMsgs: %w", err)
	}

	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &noMsg1, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &noMsg1, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}
	log.Println("VIN: " + string(rxMSGS.Data[5:22]))
	log.Println("lets waits 10s, check if adapter tester present is working, if crash then its not, otherwise read VIN again")

	time.Sleep(10 * time.Second)
	if err := pt.PassThruWriteMsgs(channelID, txMSG3, &msg1, 500); err != nil {
		return fmt.Errorf("PassThruWriteMsgs: %w", err)
	}

	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &noMsg1, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}
	if err := pt.PassThruReadMsgs(channelID, rxMSGS, &noMsg1, 900); err != nil {
		return fmt.Errorf("PassThruReadMsgs: %w", err)
	}
	log.Println("VIN: " + string(rxMSGS.Data[5:22]))

	// terminate session
	txMSG4 := &passthru.PassThruMsg{
		ProtocolID: passthru.ISO14230,
		DataSize:   5,
		Data: [4128]byte{0x80, 0x41, 0xF1, 0x01, 0x82},
	}
	if err := pt.PassThruWriteMsgs(channelID, txMSG4, &msg1, 500); err != nil {
		return fmt.Errorf("PassThruWriteMsgs: %w", err)
	}

	return nil
}
