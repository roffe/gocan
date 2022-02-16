package model

type FrameOpt func(f CANFrame)

// expect Response
func OptFrameType(frameType CANFrameType) FrameOpt {
	return func(f CANFrame) {
		switch t := f.(type) {
		case *Frame:
			t.frameType = frameType
		}
	}
}

func OptResponseRequired(f CANFrame) {
	switch t := f.(type) {
	case *Frame:
		t.frameType = ResponseRequired
	}
}
