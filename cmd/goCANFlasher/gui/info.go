package gui

import (
	"context"
	"fmt"
	"time"

	"github.com/roffe/gocan/pkg/ecu"
)

func (m *mainWindow) ecuInfo() {
	if !m.checkSelections() {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		state.inprogress = true
		defer func() {
			state.inprogress = false
		}()

		m.disableButtons()
		defer m.enableButtons()

		c, err := m.initCAN(ctx)
		if err != nil {
			m.output(err.Error())
			return
		}
		defer c.Close()

		tr, err := ecu.New(c, state.ecuType)
		if err != nil {
			m.output(err.Error())
			return
		}

		val, err := tr.Info(ctx, m.callback)
		if err != nil {
			m.output(err.Error())
		}

		for _, v := range val {
			m.output(v.String())
		}

		if err := tr.ResetECU(ctx, m.callback); err != nil {
			m.output(err.Error())
			return
		}
	}()
}

func (m *mainWindow) output(s string) {
	var text string
	if s != "" {
		text = fmt.Sprintf("%s - %s\n", time.Now().Format("15:04:05.000"), s)
	}
	//logData = append(logData, text)
	listData.Append(text)
	m.log.Refresh()
	m.log.ScrollToBottom()
}
