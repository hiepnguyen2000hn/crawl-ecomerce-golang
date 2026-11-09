package crawler

import (
	"reflect"
	"testing"
)

func containsFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func containsPair(args []string, flag, val string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == val {
			return true
		}
	}
	return false
}

func TestCommandBuilderWithComments(t *testing.T) {
	s := NewSubprocess(Config{Python: "/venv/bin/python", Script: "fetch.py"})
	_, args := s.command(FetchParams{
		URL:          "https://allegro.pl/listing?string=laptop",
		WithComments: true,
		MaxComments:  5,
	})
	if !containsFlag(args, "--with-comments") {
		t.Fatalf("expected --with-comments in args: %v", args)
	}
	if !containsPair(args, "--max-comments", "5") {
		t.Fatalf("expected --max-comments 5 in args: %v", args)
	}
}

func TestCommandBuilderWithoutComments(t *testing.T) {
	s := NewSubprocess(Config{Python: "/venv/bin/python", Script: "fetch.py"})
	_, args := s.command(FetchParams{URL: "https://allegro.pl/listing?string=laptop"})
	if containsFlag(args, "--with-comments") {
		t.Fatalf("did not expect --with-comments when disabled: %v", args)
	}
}

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
