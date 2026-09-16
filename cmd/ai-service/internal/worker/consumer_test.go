package worker

import "testing"

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{}
	err := c.HandleMessage([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestHandleMessage_JobTypeLookupError(t *testing.T) {
	// This test documents that a job-type lookup failure (e.g. unknown
	// job ID) propagates as an error without reaching Provider.Complete.
	// A real DB is required to exercise this path meaningfully; this
	// repo's established pattern (see trend-service/api-gateway tests)
	// only unit-tests pre-DB/pre-network failure paths, so this case is
	// covered instead by cmd/ai-service's manual verification against
	// the live dev database in Task 6's smoke test, not here.
	t.Skip("covered by Task 6 smoke test against a live database")
}
