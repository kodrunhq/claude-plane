package session

import (
	"log/slog"
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
