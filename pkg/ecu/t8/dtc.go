package t8

import (
	"context"
	"errors"
	"fmt"

	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) ReadDTC(ctx context.Context) ([]model.DTC, error) {
	t.gm.TesterPresentNoResponseAllowed()

	if err := t.gm.InitiateDiagnosticOperation(ctx, 0x02); err != nil {
		return nil, err
	}

	dtcs, err := t.gm.ReadDiagnosticInformationStatusOfDTCByStatusMask(ctx, 0x12)
	if err != nil {
		return nil, err
	}

	var out []model.DTC
	for _, f := range dtcs {
		//log.Printf("#%d %s", i, getDTCDescription(f))
		code, status, err := getDTCDescription(f)
		if err != nil {
			return out, err
		}
		out = append(out, model.DTC{
			Code:   code,
			Status: status,
		})
	}

	if err := t.gm.ReturnToNormalMode(ctx); err != nil {
		return out, err
	}

	return out, nil
}

// How to read DTC codes
//A7 A6    First DTC character
//-- --    -------------------
// 0  0    P - Powertrain
// 0  1    C - Chassis
// 1  0    B - Body
// 1  1    U - Network

//A5 A4    Second DTC character
//-- --    --------------------
// 0  0    0
// 0  1    1
// 1  0    2
// 1  1    3

//A3 A2 A1 A0    Third/Fourth/Fifth DTC characters
//-- -- -- --    -------------------
// 0  0  0  0    0
// 0  0  0  1    1
// 0  0  1  0    2
// 0  0  1  1    3
// 0  1  0  0    4
// 0  1  0  1    5
// 0  1  1  0    6
// 0  1  1  1    7
// 1  0  0  0    8
// 1  0  0  1    9
// 1  0  1  0    A
// 1  0  1  1    B
// 1  1  0  0    C
// 1  1  0  1    D
// 1  1  1  0    E
// 1  1  1  1    F

// Example
// E1 03 ->
// 1110 0001 0000 0011
// 11=U
//   10=2
//      0001=1
//           0000=0
//                0011=3
//----------------------
// U2103
func getDTCDescription(d []byte) (string, byte, error) {
	if len(d) != 4 {
		return "", 0, errors.New("invalid DTC bytes")
	}

	var prefix string
	switch (0xC0 & int(d[0])) >> 6 {
	case 0:
		prefix = "P"
	case 1:
		prefix = "C"
	case 2:
		prefix = "B"
	case 3:
		prefix = "U"
	default:
		prefix = "-"
	}

	one := (0x30 & int(d[0])) >> 4
	two := (0x0F & int(d[0]))
	three := (0xF0 & int(d[1])) >> 4
	four := (0x0F & int(d[1]))

	return fmt.Sprintf("%s%d%d%d%d", prefix, one, two, three, four), d[3], nil

}
