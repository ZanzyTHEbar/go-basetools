package gobasetools

import (
	"testing"
	"time"
)

// TestSubscribe ensures that subscribers can subscribe and receive messages.
// FIXME: Something here 
func TestSubscribe(t *testing.T) {
	obs := NewObservable[string]()
	defer obs.Close()

	sub, err := obs.Subscribe("testID")
	if err != nil {
		t.Fatalf("unexpected error on subscribe: %v", err)
	}

	go obs.Broadcast("Test message")

	select {
	case msg := <-sub:
		if msg != "Test message" {
			t.Errorf("expected 'Test message', got '%s'", msg)
		}
	case <-time.After(1 * time.Second):
		t.Error("did not receive message in time")
	}
}

// TestNotify tests sending a message to a specific subscriber only.
func TestNotify(t *testing.T) {
	obs := NewObservable[string]()
	defer obs.Close()

	subA, _ := obs.Subscribe("A")
	subB, _ := obs.Subscribe("B")

	go obs.Notify("A", "Hello, A only!")

	select {
	case msg := <-subA:
		if msg != "Hello, A only!" {
			t.Errorf("expected 'Hello, A only!', got '%s'", msg)
		}
	case <-subB:
		t.Error("Subscriber B should not have received the message")
	case <-time.After(1 * time.Second):
		t.Error("Subscriber A did not receive the message in time")
	}
}

// TestUnsubscribe verifies that unsubscribing stops a subscriber from receiving further messages.
func TestUnsubscribe(t *testing.T) {
	obs := NewObservable[string]()
	defer obs.Close()

	sub, _ := obs.Subscribe("testID")
	obs.Unsubscribe("testID")

	go obs.Broadcast("No message")

	select {
	case <-sub:
		t.Error("subscriber should not receive messages after unsubscribing")
	case <-time.After(100 * time.Millisecond):
		// Pass: no message was received, as expected
	}
}

// TestClose ensures all subscribers stop receiving messages after closing.
func TestClose(t *testing.T) {
	obs := NewObservable[string]()
	subA, _ := obs.Subscribe("A")
	subB, _ := obs.Subscribe("B")

	obs.Close()

	select {
	case _, open := <-subA:
		if open {
			t.Error("expected subA to be closed after Close()")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected subA to be closed after Close()")
	}

	select {
	case _, open := <-subB:
		if open {
			t.Error("expected subB to be closed after Close()")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected subB to be closed after Close()")
	}
}
