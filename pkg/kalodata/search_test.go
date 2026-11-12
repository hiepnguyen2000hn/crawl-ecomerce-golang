package kalodata

import (
	"strings"
	"testing"
)

func TestSearchArgs(t *testing.T) {
	a := Account{Email: "e@x.com", Password: "pw", Proxy: "http://u:p@h:1"}
	args := searchArgs("search.py", "USD", a, ProductParams{Country: "VN", PageNo: 2, PageSize: 10, StartDate: "s", EndDate: "e"}, "serum")
	joined := strings.Join(args, " ")
	for _, want := range []string{"--email e@x.com", "--password pw", "--country VN", "--query serum", "--pages 2", "--page-size 10", "--proxy http://u:p@h:1", "--start-date s", "--end-date e"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
}

func TestNewBrowserSearcher_FiltersLoginCapable(t *testing.T) {
	s := NewBrowserSearcher("python", "search.py", "", "",
		[]Account{{Email: "e@x.com", Password: "pw"}, {SessionCookie: "only-cookie"}})
	if len(s.accounts) != 1 {
		t.Fatalf("want 1 login-capable account, got %d", len(s.accounts))
	}
}
