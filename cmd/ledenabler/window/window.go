package window

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/gocan/adapter/j2534"
	"github.com/roffe/gocan/pkg/gmlan"
)

//go:embed ECU.png
var icon []byte
var iconRes = fyne.NewStaticResource("ECU.png", icon)

const (
	uecID = 0x64F
	recID = 0x649
)

type Main struct {
	App    fyne.App
	Window fyne.Window

	Output     *widget.List
	outputData binding.StringList

	dllSelector *widget.Select

	EnableBtn  *widget.Button
	SetBtn     *widget.Button
	GetBtn     *widget.Button
	DisableBtn *widget.Button

	uecState []byte
	recState []byte

	customUEC *widget.Entry
	customREC *widget.Entry

	portsList   []j2534.J2534DLL
	selectedDLL string
}

func NewMainWindow() *Main {

	a := app.NewWithID("com.trionictuning.hcd")
	w := a.NewWindow("Saab NG9-3 Halogen Check Disabler")

	w.SetIcon(iconRes)
	w.Resize(fyne.NewSize(600, 450))
	w.SetFixedSize(true)
	a.Settings().SetTheme(theme.DefaultTheme())

	mw := &Main{
		App:        a,
		Window:     w,
		outputData: binding.NewStringList(),
		uecState:   make([]byte, 4),
		recState:   make([]byte, 6),
	}

	w.SetContent(mw.Layout())
	mw.writeOutput("TrionicTuning Halogen Check Disabler")
	return mw
}

func (m *Main) initCAN() (*gocan.Client, error) {
	if m.selectedDLL == "" {
		return nil, fmt.Errorf("No adapter selected") //lint:ignore ST1005 ignore this error
	}
	dev, err := adapter.New(
		"j2534",
		&gocan.AdapterConfig{
			Port:         m.selectedDLL,
			PortBaudrate: 0,
			CANRate:      33.3,
			CANFilter:    []uint32{uecID, 0x649},
			Output: func(s string) {
				m.writeOutput(s)
			},
		},
	)
	if err != nil {
		return nil, err
	}
	return gocan.New(context.Background(), dev)
}

func (m *Main) disableButtons() {
	m.EnableBtn.Disable()
	m.SetBtn.Disable()
	m.DisableBtn.Disable()

}

func (m *Main) enableButtons() {
	m.EnableBtn.Enable()
	m.SetBtn.Enable()
	m.DisableBtn.Enable()
}

func (m *Main) writeOutput(s string) {
	m.outputData.Append(s)
	m.Output.Refresh()
	m.Output.ScrollToBottom()
}

func (m *Main) enableBulbOutage() {
	m.uecState[0] = 0x8b
	m.recState[0] = 0x8b
	m.setBulbOutage()
}

func (m *Main) disableBulbOutage() {
	m.uecState[0] = 0x00
	m.recState[0] = 0x00
	m.setBulbOutage()
}

func (m *Main) getBulbOutage() {
	m.disableButtons()
	defer m.enableButtons()

	c, err := m.initCAN()
	if err != nil {
		m.writeOutput(err.Error())
		return
	}
	defer c.Close()

	// UEC
	uec := gmlan.New(c, 0x24F, uecID)
	uecBytes, err := uec.ReadDataByIdentifier(context.TODO(), 0x4D)
	if err != nil {
		m.writeOutput(err.Error())
		return
	}
	m.customUEC.SetText(strings.ToUpper(hex.EncodeToString(uecBytes)))

	// REC
	rec := gmlan.New(c, 0x249, recID)
	recBytes, err := rec.ReadDataByIdentifier(context.TODO(), 0x45)
	if err != nil {
		m.writeOutput(err.Error())
		return
	}
	m.customREC.SetText(strings.ToUpper(hex.EncodeToString(recBytes)))

}

func (m *Main) setBulbOutage() {
	m.disableButtons()
	defer m.enableButtons()

	c, err := m.initCAN()
	if err != nil {
		m.writeOutput(err.Error())
		return
	}
	defer c.Close()

	m.writeOutput("Saving Registers")

	// UEC
	uec := gmlan.New(c, 0x24F, uecID)
	uecBytes, err := uec.ReadDataByIdentifier(context.TODO(), 0x4D)
	if err != nil {
		m.writeOutput(err.Error())
		return
	}
	m.writeOutput(fmt.Sprintf("Old UEC value: %X", uecBytes))

	if !bytes.Equal(m.uecState, []byte{0x00, 0x00, 0x00, 0x00}) {
		uecBytes = m.uecState
	}

	if err := uec.WriteDataByIdentifier(context.TODO(), 0x4D, uecBytes); err != nil {
		m.outputData.Append(err.Error())
		return
	}
	m.writeOutput(fmt.Sprintf("New UEC value: %X", uecBytes))

	// REC
	rec := gmlan.New(c, 0x249, recID)
	recBytes, err := rec.ReadDataByIdentifier(context.TODO(), 0x45)
	if err != nil {
		m.writeOutput(err.Error())
		return
	}
	m.writeOutput(fmt.Sprintf("Old REC value: %X", recBytes))

	if !bytes.Equal(m.recState, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) {
		recBytes = m.recState
	}

	if err := rec.WriteDataByIdentifier(context.TODO(), 0x45, recBytes); err != nil {
		m.outputData.Append(err.Error())
		return
	}
	m.writeOutput(fmt.Sprintf("New REC value: %X", recBytes))

}

func (m *Main) Layout() fyne.CanvasObject {
	m.portsList = j2534.FindDLLs()
	var adapters []string
	for _, p := range m.portsList {
		adapters = append(adapters, p.Name)
	}
	m.dllSelector = widget.NewSelect(adapters, func(s string) {
		for _, p := range m.portsList {
			if p.Name == s {
				m.selectedDLL = p.FunctionLibrary
				break
			}
		}
	})
	m.dllSelector.Alignment = fyne.TextAlignCenter

	m.EnableBtn = widget.NewButton("Enable all", m.enableBulbOutage)
	m.EnableBtn.Icon = theme.ContentAddIcon()
	m.SetBtn = widget.NewButton("Set", m.setBulbOutage)
	m.SetBtn.Icon = theme.DocumentSaveIcon()
	m.GetBtn = widget.NewButton("Get", m.getBulbOutage)
	m.GetBtn.Icon = theme.DownloadIcon()
	m.DisableBtn = widget.NewButton("Disable all", m.disableBulbOutage)
	m.DisableBtn.Icon = theme.ContentRemoveIcon()

	m.Output = widget.NewListWithData(
		m.outputData,
		func() fyne.CanvasObject {
			w := widget.NewLabel("")
			w.Alignment = fyne.TextAlignCenter
			w.TextStyle.Monospace = true
			return w
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			i := item.(binding.String)
			txt, err := i.Get()
			if err != nil {
				panic(err)
			}
			obj.(*widget.Label).SetText(txt)
		},
	)

	m.customUEC = &widget.Entry{
		PlaceHolder: strings.Repeat(" ", 8),
		OnChanged: func(s string) {
			if len(s) == 8 {
				b, err := hex.DecodeString(s)
				if err != nil {
					m.writeOutput(err.Error())
					return
				}
				m.uecState = b
			}
			if len(s) > 8 {
				m.customUEC.SetText(s[:8])
			}
		},
	}

	m.customREC = &widget.Entry{
		Wrapping: fyne.TextWrapOff,
		OnChanged: func(s string) {
			if len(s) == 12 {
				b, err := hex.DecodeString(s)
				if err != nil {
					m.writeOutput(err.Error())
					return
				}
				m.recState = b
			}
			if len(s) > 12 {
				m.customREC.SetText(s[:12])
			}
		},
	}

	uecv := widget.NewLabel("UEC Value:")
	recv := widget.NewLabel("REC Value:")
	footer := container.NewVBox(
		container.New(layout.NewGridLayout(2),
			container.NewMax(
				container.NewBorder(nil, nil, uecv, nil, m.customUEC),
			),
			container.NewMax(
				container.NewBorder(nil, nil, recv, nil, m.customREC),
			),
		),
		container.New(layout.NewGridLayout(4), m.EnableBtn, m.GetBtn, m.SetBtn, m.DisableBtn),
	)

	return container.New(layout.NewBorderLayout(
		nil,
		footer,
		nil,
		nil,
	),
		footer,
		&container.Split{
			Horizontal: true,
			Offset:     0.8,
			Leading:    m.Output,
			Trailing: container.NewVBox(
				m.dllSelector,
				widget.NewForm(
					widget.NewFormItem("Fog Lights", widget.NewCheck("", m.setUECMask(7))),
					widget.NewFormItem("High Beams", widget.NewCheck("", m.setUECMask(3))),
					widget.NewFormItem("Low Beams", widget.NewCheck("", m.setUECMask(1))),
					widget.NewFormItem("Position Lights", widget.NewCheck("", m.setUECMask(0))),
					widget.NewFormItem("Turn Signals", widget.NewCheck("", m.setRECMask(7))),
					widget.NewFormItem("Reverse Lights", widget.NewCheck("", m.setRECMask(3))),
					widget.NewFormItem("License plate", widget.NewCheck("", m.setRECMask(1))),
				),
			),
		},
	)
}

func (m *Main) setUECMask(pos uint) func(v bool) {
	return func(v bool) {
		//m.uecState = setBit(m.uecState, pos, v)
		fmt.Printf("uec: %08b\n", m.uecState)
	}
}

func (m *Main) setRECMask(pos uint) func(v bool) {
	return func(v bool) {
		//m.recState = setBit(m.recState, pos, v)
		fmt.Printf("rec: %08b\n", m.recState)
	}
}

func setBit(value byte, position uint, bitValue bool) byte {
	if position > 7 {
		panic("position must be between 0 and 7")
	}
	// Shift 1 to the left by the position to create a mask
	var mask byte = 1 << position
	// Set the bit at the given position to 1 or 0
	if bitValue {
		value |= mask // Set bit to 1
	} else {
		value &= ^mask // Set bit to 0
	}
	return value
}
