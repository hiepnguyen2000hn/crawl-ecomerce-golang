package db

import "testing"

func TestConnect_InvalidDSN(t *testing.T) {
	_, err := Connect("postgres://bad:bad@127.0.0.1:1/nonexistent?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Fatal("expected error connecting to invalid DSN, got nil")
	}
}
