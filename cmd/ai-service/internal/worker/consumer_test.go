package worker

import "testing"

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{}
	err := c.HandleMessage([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
