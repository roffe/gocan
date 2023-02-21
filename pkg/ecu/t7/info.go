package t7

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/roffe/gocan/pkg/model"
)

var T7Headers = []model.Header{
	{Desc: "VIN", ID: 0x90},
	{Desc: "Box Hardware P/N", ID: 0x91},
	{Desc: "Immo Code", ID: 0x92},
	{Desc: "Software P/N", ID: 0x94},
	{Desc: "ECU Software version:", ID: 0x95},
	{Desc: "Engine type", ID: 0x97},
	{Desc: "Tester info", ID: 0x98},
	{Desc: "Software date", ID: 0x99},
}

// Print out some Trionic7 info
func (t *Client) Info(ctx context.Context, callback model.ProgressCallback) ([]model.HeaderResult, error) {
	if err := t.DataInitialization(ctx, callback); err != nil {
		return nil, err
	}
	var out []model.HeaderResult
	for _, d := range T7Headers {
		h, err := t.GetHeader(ctx, byte(d.ID))
		if err != nil {
			return nil, fmt.Errorf("ECU info failed: %v", err)
		}
		res := model.HeaderResult{
			Value: strings.Trim(h, "\x00"),
		}
		res.Desc = d.Desc
		res.ID = d.ID
		out = append(out, res)
	}
	return out, nil
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
	res, err := t.Info(ctx, nil)
	if err != nil {
		return err
	}
	log.Println("----- ECU info ---------------")
	for _, r := range res {
		log.Println(r.Desc, r.Value)
	}
	log.Println("------------------------------")
	return nil
}
