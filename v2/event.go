package gocan

import (
	"fmt"
	"log/slog"
)

// EventType orders event severities from least to most severe.
type EventType int

const (
	EventTypeDebug EventType = iota
	EventTypeInfo
	EventTypeWarning
	EventTypeError
	// EventTypeFatal signals an unrecoverable adapter failure. It is always
	// the last event delivered before the bus terminates.
	EventTypeFatal
)

func (et EventType) String() string {
	switch et {
	case EventTypeDebug:
		return "DEBUG"
	case EventTypeInfo:
		return "INFO"
	case EventTypeWarning:
		return "WARN"
	case EventTypeError:
		return "ERROR"
	case EventTypeFatal:
		return "FATAL"
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

// Event is an out-of-band notification from an adapter: connection progress,
// recoverable errors, or the final fatal failure.
type Event struct {
	Type    EventType
	Details string
	// Err holds the underlying error for error and fatal events. It may be
	// nil for events raised from a plain message.
	Err error
}

func (e Event) String() string {
	return fmt.Sprintf("[%s] %s", e.Type, e.Details)
}

// IsFatal reports whether the event signals an unrecoverable adapter failure.
func (e Event) IsFatal() bool {
	return e.Type == EventTypeFatal
}
