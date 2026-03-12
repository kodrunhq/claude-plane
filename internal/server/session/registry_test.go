package session

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestRegistrySubscribePublish(t *testing.T) {
	r := NewRegistry(slog.Default())
	ch := r.Subscribe("s1")

	data := []byte("hello terminal")
	r.Publish("s1", data)

	select {
	case msg := <-ch:
		if string(msg.Data) != "hello terminal" {
			t.Errorf("data = %q, want %q", msg.Data, "hello terminal")
		}
		if msg.IsControl {
			t.Error("expected IsControl=false for Publish")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestRegistrySlowSubscriber(t *testing.T) {
	r := NewRegistry(slog.Default())
	ch := r.Subscribe("s1")

	// Fill the channel to capacity (256)
	for i := range 256 {
		r.Publish("s1", []byte{byte(i)})
	}

	// This should not block or panic — message should be dropped
	done := make(chan struct{})
	go func() {
		r.Publish("s1", []byte("overflow"))
		close(done)
	}()

	select {
	case <-done:
		// Success: Publish returned without blocking
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on full channel (slow subscriber)")
	}

	// Drain and verify we got 256 messages
	count := 0
	for range 256 {
		select {
		case <-ch:
			count++
		default:
		}
	}
	if count != 256 {
		t.Errorf("received %d messages, want 256", count)
	}
}

func TestRegistryDroppedCountPublish(t *testing.T) {
	r := NewRegistry(slog.Default())
	ch := r.Subscribe("s1")

	// Initially zero drops
	if got := r.DroppedCount(ch); got != 0 {
		t.Errorf("initial DroppedCount = %d, want 0", got)
	}

	// Fill the channel to capacity
	for i := range 256 {
		r.Publish("s1", []byte{byte(i)})
	}

	// Publish 5 more messages that will be dropped
	for i := range 5 {
		r.Publish("s1", []byte(fmt.Sprintf("drop-%d", i)))
	}

	if got := r.DroppedCount(ch); got != 5 {
		t.Errorf("DroppedCount after 5 drops = %d, want 5", got)
	}
}

func TestRegistryDroppedCountPublishControl(t *testing.T) {
	r := NewRegistry(slog.Default())
	ch := r.Subscribe("s1")

	// Fill the channel to capacity
	for i := range 256 {
		r.Publish("s1", []byte{byte(i)})
	}

	// Drop 3 control messages
	for i := range 3 {
		r.PublishControl("s1", []byte(fmt.Sprintf(`{"seq":%d}`, i)))
	}

	// DroppedCount should include both Publish and PublishControl drops
	if got := r.DroppedCount(ch); got != 3 {
		t.Errorf("DroppedCount after 3 control drops = %d, want 3", got)
	}
}

func TestRegistryDroppedCountUnknownChannel(t *testing.T) {
	r := NewRegistry(slog.Default())
	unknownCh := make(chan SubscriberMessage)
	if got := r.DroppedCount(unknownCh); got != 0 {
		t.Errorf("DroppedCount for unknown channel = %d, want 0", got)
	}
}

func TestRegistryDropWarnLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	r := NewRegistry(logger)
	r.Subscribe("s1")

	// Fill channel, then drop one message
	for i := range 256 {
		r.Publish("s1", []byte{byte(i)})
	}
	r.Publish("s1", []byte("dropped-data"))

	logOutput := buf.String()
	if !strings.Contains(logOutput, "dropped terminal data for slow subscriber") {
		t.Errorf("expected warn log for dropped terminal data, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "bytes=12") {
		t.Errorf("expected byte count in log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "total_dropped=1") {
		t.Errorf("expected total_dropped=1 in log, got: %s", logOutput)
	}

	// Drop a control message and verify its log too
	r.PublishControl("s1", []byte(`{"x":1}`))
	logOutput = buf.String()
	if !strings.Contains(logOutput, "dropped control message for slow subscriber") {
		t.Errorf("expected warn log for dropped control message, got: %s", logOutput)
	}
}

func TestRegistryDropCounterCumulativeAcrossMethods(t *testing.T) {
	r := NewRegistry(slog.Default())
	ch := r.Subscribe("s1")

	// Fill channel
	for i := range 256 {
		r.Publish("s1", []byte{byte(i)})
	}

	// Drop 2 via Publish, 3 via PublishControl = 5 total
	r.Publish("s1", []byte("a"))
	r.Publish("s1", []byte("b"))
	r.PublishControl("s1", []byte("c"))
	r.PublishControl("s1", []byte("d"))
	r.PublishControl("s1", []byte("e"))

	if got := r.DroppedCount(ch); got != 5 {
		t.Errorf("cumulative DroppedCount = %d, want 5", got)
	}
}

func TestRegistryUnsubscribe(t *testing.T) {
	r := NewRegistry(slog.Default())
	ch := r.Subscribe("s1")
	r.Unsubscribe("s1", ch)

	// Publishing after unsubscribe should not panic
	r.Publish("s1", []byte("after unsubscribe"))

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	default:
		t.Error("expected closed channel to be readable")
	}
}

func TestRegistryMultipleSubscribers(t *testing.T) {
	r := NewRegistry(slog.Default())
	ch1 := r.Subscribe("s1")
	ch2 := r.Subscribe("s1")

	data := []byte("broadcast")
	r.Publish("s1", data)

	for i, ch := range []chan SubscriberMessage{ch1, ch2} {
		select {
		case msg := <-ch:
			if string(msg.Data) != "broadcast" {
				t.Errorf("subscriber %d: data = %q, want %q", i, msg.Data, "broadcast")
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout", i)
		}
	}

	if r.SubscriberCount("s1") != 2 {
		t.Errorf("SubscriberCount = %d, want 2", r.SubscriberCount("s1"))
	}
}

func TestRegistryControlMessage(t *testing.T) {
	r := NewRegistry(slog.Default())
	ch := r.Subscribe("s1")

	r.PublishControl("s1", []byte(`{"type":"scrollback_end"}`))

	select {
	case msg := <-ch:
		if !msg.IsControl {
			t.Error("expected IsControl=true for PublishControl")
		}
		if string(msg.Data) != `{"type":"scrollback_end"}` {
			t.Errorf("data = %q", msg.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for control message")
	}
}
