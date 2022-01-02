package canusb

import (
	"log"
	"runtime"
	"strings"

	"go.bug.st/serial/enumerator"
)

func checkBitSet(n, k int) bool {
	v := n & (1 << (k - 1))
	return v == 1
}

func portInfo(portName string) {
	if runtime.GOOS == "windows" {
		portName = strings.ToUpper(portName)
	}
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		log.Fatal(err)
	}
	if len(ports) == 0 {
		log.Fatal("No serial ports found!")
		return
	}
	for _, port := range ports {
		if port.Name == portName {
			log.Printf("Using port: %s\n", port.Name)
			if port.IsUSB {
				log.Printf("   USB ID      %s:%s\n", port.VID, port.PID)
				log.Printf("   USB serial  %s\n", port.SerialNumber)
			}
		}
	}
}
