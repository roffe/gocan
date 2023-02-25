package legion

import (
	"context"
	"time"

	"github.com/avast/retry-go"
	"github.com/roffe/gocan/pkg/ecu/t8sec"
	"github.com/roffe/gocan/pkg/model"
)

// Disable normal communication, enter programming mode, and request security access
// then upload bootloader and jump to it

func (t *Client) Alive(ctx context.Context, callback model.ProgressCallback) bool {
	if callback != nil {
		callback("Checking if Legion is running")
	}
	err := retry.Do(func() error {
		err := t.Ping(ctx)
		if err != nil {
			return err
		}

		if callback != nil {
			callback("Legion is ready")
		}
		t.legionRunning = true
		return nil
	},
		retry.Attempts(4),
		retry.Context(ctx),
		retry.LastErrorOnly(true),
	)
	return err == nil
}

func (t *Client) Bootstrap(ctx context.Context, callback model.ProgressCallback) error {
	if !t.Alive(ctx, callback) {
		if err := t.bootstrapPreFlight(ctx, callback); err != nil {
			return err
		}
		if err := t.UploadBootloader(ctx, callback); err != nil {
			return err
		}

		if callback != nil {
			callback("Starting bootloader")
		}
		if err := t.StartBootloader(ctx, 0x102400); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		t.legionRunning = t.Alive(ctx, callback)
	}

	if t.legionRunning {
		if err := t.EnableHighSpeed(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (t *Client) bootstrapPreFlight(ctx context.Context, callback model.ProgressCallback) error {
	t.gm.TesterPresentNoResponseAllowed()

	//time.Sleep(50 * time.Millisecond)

	if err := t.gm.InitiateDiagnosticOperation(ctx, 0x02); err != nil {
		return err
	}

	if err := t.gm.DisableNormalCommunication(ctx); err != nil {
		return err
	}

	if err := t.gm.ReportProgrammedState(ctx); err != nil {
		return err
	}

	if err := t.gm.ProgrammingModeRequest(ctx); err != nil {
		return err
	}

	if err := t.gm.ProgrammingModeEnable(ctx); err != nil {
		return err
	}

	time.Sleep(50 * time.Millisecond)

	t.gm.TesterPresentNoResponseAllowed()

	if callback != nil {
		callback("Requesting security access")
	}
	if err := t.gm.RequestSecurityAccess(ctx, 0x01, 0, t8sec.CalculateAccessKey); err != nil {
		return err
	}

	time.Sleep(50 * time.Millisecond)
	return nil
}
