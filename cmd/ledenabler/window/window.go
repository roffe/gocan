package window

import (
	"context"
	_ "embed"
	"encoding/hex"
	"fmt"
	"log"
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

//go:embed hk_white.png
var icon []byte
var iconRes = fyne.NewStaticResource("hk_white.png", icon)

type Main struct {
	App    fyne.App
	Window fyne.Window

	Output     *widget.List
	outputData binding.StringList

	dllSelector *widget.Select

	GetBtn *widget.Button
	SetBtn *widget.Button

	uecInput *widget.Entry
	recInput *widget.Entry

	uecCheckFreq binding.Float
	recCheckFreq binding.Float

	calculatedUECFreq *widget.Label
	calculatedRECFreq *widget.Label

	portsList   []j2534.J2534DLL
	selectedDLL string

	uecFoglightsCheckbox   *widget.Check
	uecDippedBeamsCheckbox *widget.Check
	uecHighBeamsCheckbox   *widget.Check
	uecTurnSignalsCheckbox *widget.Check

	ub []byte
	rb []byte
}

const timePerTick = 14.5

func freqToSeconds(f float64) float64 {
	return timePerTick * f / 1000
}

func NewMainWindow() *Main {

	a := app.NewWithID("com.trionictuning.hcd")
	w := a.NewWindow("Saab NG9-3 Halogen Check Disabler")

	w.SetIcon(iconRes)
	w.Resize(fyne.NewSize(850, 450))
	//w.SetFixedSize(true)
	a.Settings().SetTheme(&MyTheme{})

	m := &Main{
		App:        a,
		Window:     w,
		outputData: binding.NewStringList(),

		uecInput:          widget.NewEntry(),
		recInput:          widget.NewEntry(),
		uecCheckFreq:      binding.NewFloat(),
		recCheckFreq:      binding.NewFloat(),
		calculatedUECFreq: widget.NewLabel("0s"),
		calculatedRECFreq: widget.NewLabel("0s"),
	}

	m.uecFoglightsCheckbox = widget.NewCheck("", m.setUECMask(6))
	m.uecDippedBeamsCheckbox = widget.NewCheck("", m.setUECMask(5))
	m.uecHighBeamsCheckbox = widget.NewCheck("", m.setUECMask(4))
	m.uecTurnSignalsCheckbox = widget.NewCheck("", m.setUECMask(0))

	m.uecCheckFreq.AddListener(binding.NewDataListener(func() {
		f, err := m.uecCheckFreq.Get()
		if err != nil {
			m.writeOutput(err.Error())
			return
		}
		if len(m.ub) > 0 {
			m.ub[0] = byte(f)
		}
		m.calculatedUECFreq.SetText(fmt.Sprintf("%.02fs", freqToSeconds(f)))
		m.uecInput.SetText(strings.ToUpper(hex.EncodeToString(m.ub)))
	}))

	m.recCheckFreq.AddListener(binding.NewDataListener(func() {
		f, err := m.recCheckFreq.Get()
		if err != nil {
			m.writeOutput(err.Error())
			return
		}
		if len(m.rb) > 0 {
			m.rb[0] = byte(f)
		}
		m.calculatedRECFreq.SetText(fmt.Sprintf("%.02fs", freqToSeconds(f)))
		m.recInput.SetText(strings.ToUpper(hex.EncodeToString(m.rb)))
	}))

	m.uecInput.OnChanged = func(s string) {
		if len(s)%2 == 0 {
			uecData, err := hex.DecodeString(s)
			if err != nil {
				m.writeOutput(err.Error())
				return
			}
			m.setInternalUECState(uecData)
		}
	}

	m.recInput.OnChanged = func(s string) {
		if len(s)%2 == 0 {
			recData, err := hex.DecodeString(s)
			if err != nil {
				m.writeOutput(err.Error())
				return
			}
			m.setInternalRECState(recData)
		}
	}

	w.SetContent(m.Layout())
	//m.writeOutput("TrionicTuning Halogen Check Disabler")
	return m
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
			CANFilter:    []uint32{0x64f, 0x649},
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
	m.GetBtn.Disable()
	m.SetBtn.Disable()

}

func (m *Main) enableButtons() {
	m.GetBtn.Enable()
	m.SetBtn.Enable()
}

func (m *Main) writeOutput(s string) {
	m.outputData.Append(s)
	m.Output.Refresh()
	m.Output.ScrollToBottom()
}

func (m *Main) getStates() error {
	m.disableButtons()
	defer m.enableButtons()

	c, err := m.initCAN()
	if err != nil {
		return err
	}
	defer c.Close()

	// UEC
	uec := gmlan.New(c, 0x24F, 0x64f)

	vin, err := uec.ReadDataByIdentifierString(context.TODO(), 0x90)
	if err != nil {
		return err
	}

	m.writeOutput(fmt.Sprintf("VIN: %s", vin))

	uecBytes, err := uec.ReadDataByIdentifier(context.TODO(), 0x4D)
	if err != nil {
		return err
	}
	m.setInternalUECState(uecBytes)

	// REC
	rec := gmlan.New(c, 0x249, 0x649)
	recBytes, err := rec.ReadDataByIdentifier(context.TODO(), 0x45)
	if err != nil {
		return err
	}
	m.setInternalRECState(recBytes)

	return nil
}

func (m *Main) setInternalUECState(data []byte) {
	if len(data) < 4 {
		return
	}
	m.ub = data
	if data[1]&0x80 == 0x80 {
	}
	if data[1]&0x40 == 0x40 {
		m.uecFoglightsCheckbox.SetChecked(true)
	} else {
		m.uecFoglightsCheckbox.SetChecked(false)
	}
	if data[1]&0x20 == 0x20 {
		m.uecDippedBeamsCheckbox.SetChecked(true)
	} else {
		m.uecDippedBeamsCheckbox.SetChecked(false)
	}
	if data[1]&0x10 == 0x10 {
		m.uecHighBeamsCheckbox.SetChecked(true)
	} else {
		m.uecHighBeamsCheckbox.SetChecked(false)
	}
	if data[1]&0x01 == 0x01 {
		m.uecTurnSignalsCheckbox.SetChecked(true)
	} else {
		m.uecTurnSignalsCheckbox.SetChecked(false)
	}
	if err := m.uecCheckFreq.Set(float64(data[0])); err != nil {
		m.writeOutput(err.Error())
	}
	m.uecInput.SetText(strings.ToUpper(hex.EncodeToString(m.ub)))
}

func (m *Main) setInternalRECState(data []byte) {
	if len(data) != 6 {
		return
	}
	m.rb = data
	if err := m.recCheckFreq.Set(float64(m.rb[0])); err != nil {
		m.writeOutput(err.Error())
	}
	m.recInput.SetText(strings.ToUpper(hex.EncodeToString(m.rb)))
}

func (m *Main) setState() {
	m.disableButtons()
	defer m.enableButtons()

	c, err := m.initCAN()
	if err != nil {
		m.writeOutput(err.Error())
		return
	}
	defer c.Close()

	// UEC
	uec := gmlan.New(c, 0x24F, 0x64f)
	if err := uec.WriteDataByIdentifier(context.TODO(), 0x4D, m.ub); err != nil {
		m.outputData.Append("UEC: " + err.Error())
		return
	}

	// REC
	rec := gmlan.New(c, 0x249, 0x649)
	if err := rec.WriteDataByIdentifier(context.TODO(), 0x45, m.rb); err != nil {
		m.outputData.Append("REC: " + err.Error())
		return
	}

	m.writeOutput("Set state successful")
}

const prefsSelectedDLLKey = "selectedDLL"

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

	if selected := m.App.Preferences().String(prefsSelectedDLLKey); selected != "" {
		m.dllSelector.SetSelected(selected)
	}

	m.dllSelector.OnChanged = func(s string) {
		m.App.Preferences().SetString(prefsSelectedDLLKey, s)
	}

	m.SetBtn = widget.NewButton("Save configuration", m.setState)
	m.SetBtn.Icon = theme.DocumentSaveIcon()
	m.GetBtn = widget.NewButton("Load configuration", func() {
		if err := m.getStates(); err != nil {
			m.writeOutput(err.Error())
		}
	})
	m.GetBtn.Icon = theme.DownloadIcon()

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

	footer := container.NewVBox(
		container.New(layout.NewGridLayout(2),
			container.NewMax(
				container.NewBorder(nil, nil, widget.NewLabel("UEC check interval:"), m.calculatedUECFreq, widget.NewSliderWithData(0, 255, m.uecCheckFreq)),
				widget.NewLabel(""),
			),
			container.NewMax(
				container.NewBorder(nil, nil, widget.NewLabel("REC check interval:"), m.calculatedRECFreq, widget.NewSliderWithData(0, 255, m.recCheckFreq)),
			),
		),
		container.New(layout.NewGridLayout(2),
			container.NewMax(
				container.NewBorder(nil, nil, widget.NewLabel("UEC value:"), nil, m.uecInput),
			),
			container.NewMax(
				container.NewBorder(nil, nil, widget.NewLabel("REC value:"), nil, m.recInput),
			),
		),
		container.New(layout.NewGridLayout(2), m.GetBtn, m.SetBtn),
	)

	/*
		80-???
		40-fog lights
		20-dipped beam
		10-main beam
		01-direction indicators (all)
	*/

	return container.NewBorder(
		nil,
		footer,
		nil,
		nil,
		&container.Split{
			Horizontal: true,
			Offset:     0.8,
			Leading:    m.Output,
			Trailing: container.NewVBox(
				m.dllSelector,
				widget.NewForm(
					// UEC
					widget.NewFormItem("Fog Lights", m.uecFoglightsCheckbox),
					widget.NewFormItem("Dipped Beams", m.uecDippedBeamsCheckbox),
					widget.NewFormItem("High Beams", m.uecHighBeamsCheckbox),
					widget.NewFormItem("Turn Signals (all)", m.uecTurnSignalsCheckbox),
					// REC
					widget.NewFormItem("Position Lights", widget.NewCheck("", m.setUECMask(0))),
					widget.NewFormItem("Reverse Lights", widget.NewCheck("", m.setRECMask(3))),
					widget.NewFormItem("License plate", widget.NewCheck("", m.setRECMask(1))),
				),
			),
		},
	)
}

func (m *Main) setUECMask(pos uint) func(v bool) {
	return func(v bool) {
		if len(m.ub) < 2 {
			return
		}
		m.ub[1] = setBit(m.ub[1], pos, v)
		m.uecInput.SetText(strings.ToUpper(hex.EncodeToString(m.ub)))
	}
}

func (m *Main) setRECMask(pos uint) func(v bool) {
	return func(v bool) {
		//m.recState = setBit(m.recState, pos, v)
		log.Printf("rec: %08b %02X\n", m.rb, m.rb)
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
