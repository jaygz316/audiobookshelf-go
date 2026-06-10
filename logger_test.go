package main

import (
	"strconv"
	"testing"
)

func TestLogBufferCircular(t *testing.T) {
	lb := NewLogBuffer(5)

	// Add 3 items
	for i := 1; i <= 3; i++ {
		lb.Add(LogMessage{Message: "msg" + strconv.Itoa(i)})
	}

	msgs := lb.Get()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Message != "msg1" || msgs[1].Message != "msg2" || msgs[2].Message != "msg3" {
		t.Errorf("unexpected message order: %v", msgs)
	}

	// Fill buffer to max capacity (5 items)
	lb.Add(LogMessage{Message: "msg4"})
	lb.Add(LogMessage{Message: "msg5"})

	msgs = lb.Get()
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}
	if msgs[0].Message != "msg1" || msgs[4].Message != "msg5" {
		t.Errorf("unexpected message order: %v", msgs)
	}

	// Add 2 more items to trigger eviction/circular overwrite
	lb.Add(LogMessage{Message: "msg6"}) // should overwrite msg1
	lb.Add(LogMessage{Message: "msg7"}) // should overwrite msg2

	msgs = lb.Get()
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}

	expected := []string{"msg3", "msg4", "msg5", "msg6", "msg7"}
	for i, exp := range expected {
		if msgs[i].Message != exp {
			t.Errorf("expected index %d to be %q, got %q", i, exp, msgs[i].Message)
		}
	}
}
