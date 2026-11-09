package worker

import (
	"encoding/json"
	"testing"
)

func TestCommentsDefaults(t *testing.T) {
	var m JobMessage
	if err := json.Unmarshal([]byte(`{"job_id":"1"}`), &m); err != nil {
		t.Fatal(err)
	}
	if !commentsEnabled(m) {
		t.Fatal("default should enable comments when with_comments absent")
	}
	if commentsLimit(m) != 5 {
		t.Fatalf("default limit want 5 got %d", commentsLimit(m))
	}

	var m2 JobMessage
	if err := json.Unmarshal([]byte(`{"job_id":"1","with_comments":false,"max_comments":3}`), &m2); err != nil {
		t.Fatal(err)
	}
	if commentsEnabled(m2) {
		t.Fatal("explicit with_comments=false should disable")
	}
	if commentsLimit(m2) != 3 {
		t.Fatalf("explicit limit want 3 got %d", commentsLimit(m2))
	}
}

func TestResolveTargets(t *testing.T) {
	c := &Consumer{}
	got := c.resolveTargets(JobMessage{JobID: "1", Keyword: "laptop gamingowy"})
	if len(got) != 1 || got[0] != "https://allegro.pl/listing?string=laptop+gamingowy" {
		t.Fatalf("unexpected keyword target: %v", got)
	}

	got2 := c.resolveTargets(JobMessage{JobID: "2", URL: "https://allegro.pl/listing?string=dell"})
	if len(got2) != 1 || got2[0] != "https://allegro.pl/listing?string=dell" {
		t.Fatalf("unexpected url target: %v", got2)
	}
}

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{}
	if err := c.HandleMessage([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestHandleMessage_EmptyTargetsError(t *testing.T) {
	c := &Consumer{}
	if err := c.HandleMessage([]byte(`{"job_id":"123"}`)); err == nil {
		t.Fatal("expected error when neither keyword nor url is provided")
	}
}
