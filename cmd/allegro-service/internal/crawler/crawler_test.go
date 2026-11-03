package crawler

import (
	"reflect"
	"testing"
)

func TestCommandBuilder(t *testing.T) {
	s := NewSubprocess(Config{
		Python: "/venv/bin/python",
		Script: "cmd/allegro-service/crawler/fetch.py",
		Xvfb:   true,
	})
	bin, args := s.command(FetchParams{
		URL:      "https://allegro.pl/listing?string=laptop",
		Pages:    2,
		MaxItems: 50,
	})
	if bin != "xvfb-run" {
		t.Fatalf("expected xvfb-run, got %s", bin)
	}
	want := []string{
		"-a",
		"/venv/bin/python",
		"cmd/allegro-service/crawler/fetch.py",
		"https://allegro.pl/listing?string=laptop",
		"--pages", "2",
		"--max-items", "50",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected args:\ngot:  %v\nwant: %v", args, want)
	}
}
