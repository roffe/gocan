package gocan

import (
	"fmt"
	"log/slog"
)

type EventType int

const (
	EventTypeError EventType = iota
	EventTypeWarning
	EventTypeInfo
	EventTypeDebug
	// EventTypeFatal signals an unrecoverable adapter failure. It is always
	// the last event delivered before the client terminates.
	EventTypeFatal
)

func (et EventType) String() string {
	switch et {
	case EventTypeFatal:
		return "FATAL"
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

// Level maps the event type to the matching slog level so events can be
// forwarded to an slog.Logger with WithLogger.
func (et EventType) Level() slog.Level {
	switch et {
	case EventTypeFatal, EventTypeError:
		return slog.LevelError
	case EventTypeWarning:
		return slog.LevelWarn
	case EventTypeInfo:
		return slog.LevelInfo
	default:
		return slog.LevelDebug
	}
}

type Event struct {
	Type    EventType
	Details string
	// Err holds the underlying error for EventTypeError and EventTypeFatal
	// events. It may be nil for events raised from a plain message.
	Err error
}

// Returns a formatted string representation of the event.
func (e Event) String() string {
	return fmt.Sprintf("[%s] %s", e.Type.String(), e.Details)
}

// Returns the raw details of the event.
func (e Event) Raw() string {
	return e.Details
}

// IsFatal reports whether the event signals an unrecoverable adapter failure.
func (e Event) IsFatal() bool {
	return e.Type == EventTypeFatal
}
