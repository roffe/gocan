package gocan

import "fmt"

type EventHandler struct {
	Type    EventType
	Handler func(Event)
}

type EventType int

func (et EventType) String() string {
	switch et {
	case EventTypeError:
		return "ERROR"
	case EventTypeWarning:
		return "WARN"
	case EventTypeInfo:
		return "INFO"
	case EventTypeDebug:
		return "DEBUG"
	default:
		return "UNKNOWN"
	}
}

const (
	EventTypeError EventType = iota
	EventTypeWarning
	EventTypeInfo
	EventTypeDebug
)

type Event struct {
	Type    EventType
	Details string
}

func (e Event) String() string {
	return fmt.Sprintf("[%s] %s", e.Type.String(), e.Details)
}
