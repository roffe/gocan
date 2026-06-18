package gocan

import (
	"testing"
	"time"

	gwpb "github.com/roffe/gocan/proto"
	googleproto "google.golang.org/protobuf/proto"
)

// The flattened CANFrame and the StreamMessage oneof survive a marshal round-trip.
func TestStreamMessageRoundTrip(t *testing.T) {
	frameMsg := &gwpb.StreamMessage{Payload: &gwpb.StreamMessage_Frame{Frame: &gwpb.CANFrame{
		Id:        0x7E8,
		Data:      []byte{0x01, 0x02, 0x03},
		FrameType: gwpb.CANFrameTypeEnum_Incoming,
		Responses: 2,
	}}}
	b, err := googleproto.Marshal(frameMsg)
	if err != nil {
		t.Fatal(err)
	}
	var gotFrame gwpb.StreamMessage
	if err := googleproto.Unmarshal(b, &gotFrame); err != nil {
		t.Fatal(err)
	}
	f := gotFrame.GetFrame()
	if f == nil {
		t.Fatal("expected frame payload")
	}
	if f.GetId() != 0x7E8 || f.GetResponses() != 2 || f.GetFrameType() != gwpb.CANFrameTypeEnum_Incoming {
		t.Fatalf("frame round-trip mismatch: %+v", f)
	}

	eventMsg := &gwpb.StreamMessage{Payload: &gwpb.StreamMessage_Event{Event: &gwpb.Event{
		Level:   gwpb.EventLevel_EVENT_WARN,
		Message: "careful",
	}}}
	eb, err := googleproto.Marshal(eventMsg)
	if err != nil {
		t.Fatal(err)
	}
	var gotEvent gwpb.StreamMessage
	if err := googleproto.Unmarshal(eb, &gotEvent); err != nil {
		t.Fatal(err)
	}
	ev := gotEvent.GetEvent()
	if ev == nil || ev.GetLevel() != gwpb.EventLevel_EVENT_WARN || ev.GetMessage() != "careful" {
		t.Fatalf("event round-trip mismatch: %+v", gotEvent.GetEvent())
	}
}

// GWClient translates downstream StreamMessage payloads into frames and
// level-preserving events, with fatal events routed to the error channel.
func TestGWClientDeliver(t *testing.T) {
	c, err := NewGWClient("test", &AdapterConfig{})
	if err != nil {
		t.Fatal(err)
	}

	c.deliverFrame(&gwpb.CANFrame{Id: 0x123, Data: []byte{0x09}, FrameType: gwpb.CANFrameTypeEnum_Incoming})
	select {
	case fr := <-c.Recv():
		if fr.Identifier != 0x123 || len(fr.Data) != 1 || fr.Data[0] != 0x09 {
			t.Fatalf("unexpected frame: %+v", fr)
		}
	case <-time.After(time.Second):
		t.Fatal("no frame delivered")
	}

	c.deliverEvent(&gwpb.Event{Level: gwpb.EventLevel_EVENT_WARN, Message: "w"})
	select {
	case ev := <-c.Event():
		if ev.Type != EventTypeWarning || ev.Details != "w" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no event delivered")
	}

	c.deliverEvent(&gwpb.Event{Level: gwpb.EventLevel_EVENT_FATAL, Message: "boom"})
	select {
	case err := <-c.Err():
		if err == nil || err.Error() != "boom" {
			t.Fatalf("unexpected fatal: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("no fatal delivered")
	}
}
