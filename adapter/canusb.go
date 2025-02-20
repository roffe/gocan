//go:build canusb

package adapter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/roffe/gocan"
	canusb "github.com/roffe/gocanusb"
)

func init() {
	if names, err := canusb.GetAdapters(); err == nil {
		for _, name := range names {
			nameStr := fmt.Sprintf("CANUSB %s", name)
			if err := Register(&AdapterInfo{
				Name:               nameStr,
				New:                NewCanusbDLLName(nameStr),
				RequiresSerialPort: false,
				Capabilities: AdapterCapabilities{
					HSCAN: true,
					KLine: false,
					SWCAN: false,
				},
			}); err != nil {
				panic(err)
			}
		}
	}
}

type CanusbDLL struct {
	BaseAdapter

	h *canusb.CANHANDLE

	closeOnce sync.Once
}

func NewCanusbDLLName(name string) func(*gocan.AdapterConfig) (gocan.Adapter, error) {
	return func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
		return NewCanusbDLL(name, cfg)
	}
}

func NewCanusbDLL(name string, cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &CanusbDLL{
		BaseAdapter: NewBaseAdapter(name, cfg),
	}, nil
}

func (cu *CanusbDLL) getRate(rate float64) string {
	switch rate {
	case 33.3:
		return "0x0e:0x1c"
	case 47.619:
		return "0xcb:0x9a"
	case 615.384:
		return "0x40:0x37"
	default:
		return strconv.FormatFloat(rate, 'f', 1, 64)
	}
}

func (cu *CanusbDLL) calculateFilterMask(canIDs []uint32, isExtended bool) (uint32, uint32) {
	if len(canIDs) == 0 {
		// No IDs provided, accept all messages
		return 0, 0xFFFFFFFF
	}

	// Start with the first ID as reference
	referenceID := canIDs[0]

	// Initialize mask with all zeros (all bits must match initially)
	mask := uint32(0)

	// For multiple IDs, find bits that differ
	for _, id := range canIDs {
		mask |= (referenceID ^ id)
	}

	var code, finalMask uint32

	if isExtended {
		// Extended frame (29-bit identifier)
		// Ensure we only use the valid 29 bits
		referenceID &= 0x1FFFFFFF
		mask &= 0x1FFFFFFF

		// Important: SJA1000 uses first 29 bits for extended ID filtering
		// For acceptance code register (ACR)
		code = referenceID

		// For acceptance mask register (AMR)
		// In SJA1000, 0 in mask means "bit must match", 1 means "don't care"
		finalMask = mask

		// Most SJA1000 implementations require specific alignment for extended frames
		// No need to set IDE bit here, as it's handled differently in extended mode
	} else {
		// Standard frame (11-bit identifier)
		// Ensure we only use the valid 11 bits
		referenceID &= 0x7FF
		mask &= 0x7FF

		// For standard frames, SJA1000 typically uses bits 20-30 for the ID
		// with specific bit alignment for the acceptance filter
		code = referenceID << 5

		// Standard frame mask
		finalMask = mask << 5

		// For standard frames, bits 0-4 are typically RTR and control bits
		// and remaining high bits don't participate in filtering
		finalMask |= 0xFFFFE01F
	}

	if cu.cfg.Debug {
		log.Printf("code: %08X mask: %08X", code, finalMask)
	}
	return code, finalMask
}

func (cu *CanusbDLL) Connect(ctx context.Context) error {
	parts := strings.Split(cu.name, " ")
	if len(parts) != 2 {
		return fmt.Errorf("invalid adapter name %q", cu.name)
	}

	code, mask := cu.calculateFilterMask(cu.cfg.CANFilter, false)

	var err error
	cu.h, err = canusb.Open(
		parts[1],
		cu.getRate(cu.cfg.CANRate),
		code,                      //canusb.ACCEPTANCE_CODE_ALL,
		mask,                      //canusb.ACCEPTANCE_MASK_ALL,
		canusb.FLAG_NO_LOCAL_SEND, //|canusb.FLAG_BLOCK|canusb.FLAG_TIMESTAMP|canusb.FLAG_SLOW,
	)
	if err != nil {
		return err
	}

	if cu.cfg.PrintVersion {
		ver, err := cu.h.VersionInfo()
		if err != nil {
			cu.SetError(fmt.Errorf("get version failed: %w", err))
		} else {
			cu.cfg.OnMessage(ver)
		}
	}

	if err := cu.h.SetReceiveCallback(cu.callbackHandler); err != nil {
		cu.h.Close()
		return err
	}

	go cu.run(ctx)

	return nil
}

func (cu *CanusbDLL) callbackHandler(msg *canusb.CANMsg) uintptr {
	// copy the data as the callback will overwrite it when returning
	data := make([]byte, msg.Len)
	copy(data, msg.Data[:msg.Len])
	select {
	case cu.recvChan <- gocan.NewFrame(msg.Id, data, gocan.Incoming):
	default:
		cu.SetError(errors.New("recvChan full, dropping frame"))
	}
	return 0
}

func (cu *CanusbDLL) run(ctx context.Context) {
	stats := time.NewTicker(10 * time.Second)
	defer stats.Stop()
	if !cu.cfg.Debug {
		stats.Stop()
	}
	status := time.NewTicker(2 * time.Second)
	defer status.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cu.closeChan:
			return
		case <-stats.C:
			st, err := cu.h.GetStatistics()
			if err != nil {
				cu.SetError(fmt.Errorf("get statistics failed: %w", err))
				continue
			}
			log.Println(st.String())
		case <-status.C:
			if err := cu.h.Status(); err != nil {
				cu.SetError(err)
				continue
			}
		case frame := <-cu.sendChan:
			if frame.Identifier() >= gocan.SystemMsg {
				continue
			}
			var data [8]byte
			copy(data[:], frame.Data())
			msg := &canusb.CANMsg{
				Id:   frame.Identifier(),
				Len:  uint8(len(frame.Data())),
				Data: data,
			}
			if err := cu.h.Write(msg); err != nil {
				cu.SetError(fmt.Errorf("write failed: %w", err))
			}
		}
	}
}

func (cu *CanusbDLL) SetFilter(filters []uint32) error {
	return nil
}

func (cu *CanusbDLL) Close() (err error) {
	cu.BaseAdapter.Close()
	cu.closeOnce.Do(func() {
		if err = cu.h.Flush(canusb.FLUSH_EMPTY_INQUEUE | canusb.FLUSH_WAIT); err != nil {
			log.Println("Flush:", err)
		}
		if err = cu.h.Close(); err != nil {
			log.Println("Close:", err)
		}
		cu.h = nil
	})
	return
}
