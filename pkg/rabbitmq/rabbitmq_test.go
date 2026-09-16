package rabbitmq

import "testing"

func TestDLQName(t *testing.T) {
	got := dlqName("trend.jobs", ".dlq")
	want := "trend.jobs.dlq"
	if got != want {
		t.Fatalf("dlqName() = %q, want %q", got, want)
	}
}
