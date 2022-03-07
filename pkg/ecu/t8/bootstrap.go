package t8

import (
	"context"
	"time"

	"github.com/avast/retry-go"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) Bootstrap(ctx context.Context, callback model.ProgressCallback) error {
	var legionRunning bool
	if callback != nil {
		callback("Checking if Legion is running")
	}
	retry.Do(func() error {
		err := t.LegionPing(ctx)
		if err != nil {
			return err
		}

		if callback != nil {
			callback("Legion running")
		}
		legionRunning = true
		return nil
	},
		retry.Attempts(4),
		retry.Context(ctx),
		retry.LastErrorOnly(true),
	)
	gm := gmlan.New(t.c)
	gm.TesterPresentNoResponseAllowed()

	if err := gm.InitiateDiagnosticOperation(ctx, 0x02, 0x7E0, 0x7E8); err != nil {
		return err
	}

	if err := gm.DisableNormalCommunication(ctx, 0x7E0, 0x7E8); err != nil {
		return err
	}

	if err := gm.ReportProgrammedState(ctx, 0x7E0, 0x7E8); err != nil {
		return err
	}

	if err := gm.ProgrammingModeRequest(ctx, 0x7E0, 0x7E8); err != nil {
		return err
	}

	if err := gm.ProgrammingModeEnable(ctx, 0x7E0, 0x7E8); err != nil {
		return err
	}

	time.Sleep(50 * time.Millisecond)

	gm.TesterPresentNoResponseAllowed()

	if callback != nil {
		callback("Requesting security access")
	}
	if err := t.RequestSecurityAccess(ctx, 0x01, 0); err != nil {
		return err
	}

	if !legionRunning {
		if err := t.UploadBootloader(ctx, callback); err != nil {
			return err
		}

		if callback != nil {
			callback("Start bootloader")
		}
		if err := t.StartBootloader(ctx, 0x102400); err != nil {
			return err
		}

		time.Sleep(500 * time.Millisecond)

		if callback != nil {
			callback("Checking if Legion is running")
		}

		err := retry.Do(func() error {
			return t.LegionPing(ctx)
		},
			retry.Attempts(4),
			retry.Context(ctx),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return err
		}
	}

	return nil
}
