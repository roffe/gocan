package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"
	"unsafe"

	"github.com/roffe/gocan/pkg/pcan"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	//s := pcan.CAN_GetValue(pcan.PCAN_USBBUS1, pcan.PCAN_CHANNEL_CONDITION, uintptr(unsafe.Pointer(&condition)), 4)
	//log.Println(s)
	//log.Printf("Channel condition: 0x%X\n", condition)

	ch := pcan.PCAN_USBBUS1
	/*
		log.Println("Identifying channel", ch)
		pcan.SetChannelIdentifying(ch, true)
		time.Sleep(2 * time.Second)
		log.Println("Stopping identifying channel", ch)
		pcan.SetChannelIdentifying(ch, false)
		time.Sleep(2 * time.Second)
	*/

	//text := pcan.GetErrorText(pcan.PCAN_ERROR_OVERRUN)
	//log.Printf("Error text: %s\n", text)
	//return

	deviceID, err := pcan.GetDeviceID(ch)
	if err != nil {
		log.Println(err)
	}
	log.Printf("Device ID: 0x%X\n", deviceID)

	hwName, err := pcan.GetHardwareName(ch)
	if err != nil {
		log.Println(err)
	}
	log.Printf("Hardware Name: %s\n", hwName)

	partNumber, err := pcan.GetDevicePartNumber(ch)
	if err != nil {
		log.Println(err)
	}
	log.Printf("Part Number: %s\n", partNumber)

	guid, err := pcan.GetDeviceGUID(ch)
	if err != nil {
		log.Println(err)
	}
	log.Printf("Device GUID: %s\n", guid)

	apiVersion, err := pcan.GetAPIVersion()
	if err != nil {
		log.Println(err)
	}
	log.Printf("API Version: %s\n", apiVersion)

	channelVersion, err := pcan.GetChannelVersion(ch)
	if err != nil {
		log.Println(err)
	}
	log.Printf("Channel Version: %s\n", channelVersion)

	channelFeatures, err := pcan.GetChannelFeatures(ch)
	if err != nil {
		log.Println(err)
	}
	log.Printf("Channel Features: 0x%X\n", channelFeatures)

	cond := pcan.GetChannelCondition(ch)
	log.Printf("Channel %02X: %s\n", ch, cond.String())

	channels, err := pcan.GetAttachedChannelsCount()
	if err != nil {
		log.Println(err)
	}
	log.Printf("Attached Channels Count: %d\n", len(channels))
	for _, channel := range channels {
		log.Printf("- Name: %s", channel.DeviceName)
		log.Printf("  Handle:     0x%02X", channel.ChannelHandle)
		log.Printf("  Controller: %d", channel.ControllerNumber)
		log.Printf("  Features:   %d", channel.DeviceFeatures)
		log.Printf("  Device ID:  %d", channel.DeviceID)
		log.Printf("  Condition:  %d", channel.ChannelCondition)
	}

	// Initialize the channel
	if err := pcan.CAN_Initialize(ch, pcan.PCAN_BAUD_500K); err != nil {
		log.Fatal("Error initializing channel:", err)
	} else {
		log.Println("Channel initialized successfully")
	}

	firmwareVersion, err := pcan.GetFirmwareVersion(ch)
	if err != nil {
		log.Println(err)
	}

	log.Printf("Firmware Version: %s\n", firmwareVersion)
	param := pcan.PCAN_PARAMETER_ON
	if err := pcan.CAN_SetValue(ch, pcan.PCAN_ALLOW_ERROR_FRAMES, uintptr(unsafe.Pointer(&param)), 4); err != nil {
		log.Println("Error setting ALLOW_ERROR_FRAMES:", err)
	} else {
		log.Println("ALLOW_ERROR_FRAMES set to ON")
	}

	if err := pcan.FilterMessages(ch, 0x05, 0x0C, pcan.PCAN_MODE_STANDARD); err != nil {
		log.Println("Error setting message filter:", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, ts, err := pcan.Read(ch)
				if err != nil {
					if err.(pcan.PCANError).Code == pcan.PCAN_ERROR_QRCVEMPTY {
						time.Sleep(200 * time.Microsecond)
						continue
					}
					log.Println(err)
					continue
				}
				log.Printf("Received message: ID=0x%X LEN=%d DATA=% X TIMESTAMP=%d.%03d\n", msg.ID, msg.LEN, msg.DATA[:msg.LEN], ts.Millis, ts.Micros)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				err := pcan.Write(ch, &pcan.TPCANMsg{
					ID:   0x123,
					LEN:  8,
					DATA: [8]byte{0, 1, 2, 3, 4, 5, 6, 7},
				})
				if err != nil {
					log.Println("Error writing message:", err)
				}
			}
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")

	if err := pcan.CAN_Uninitialize(ch); err != nil {
		log.Println("Error uninitializing channel:", err)
	} else {
		log.Println("Channel uninitialized successfully")
	}

}
