package kalodata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"sync/atomic"
)

// ErrNoSearchAccounts means no login-capable account (email+password) is
// configured for browser keyword search.
var ErrNoSearchAccounts = errors.New("kalodata: no login-capable accounts for keyword search")

const (
	searchExitOK        = 0
	searchExitLoginFail = 2
	searchExitUsage     = 4
)

// BrowserSearcher runs keyword search through a real browser login (Camoufox),
// because /product/searchList rejects injected-cookie sessions. It shells out to
// crawler/search.py, rotating across login-capable accounts.
type BrowserSearcher struct {
	python   string
	script   string
	cdnBase  string
	currency string
	accounts []Account
	next     uint64
}

func NewBrowserSearcher(python, script, cdnBase, currency string, accounts []Account) *BrowserSearcher {
	if cdnBase == "" {
		cdnBase = defaultCDNBase
	}
	if currency == "" {
		currency = "USD"
	}
	// Only accounts with email+password can log in.
	var usable []Account
	for _, a := range accounts {
		if a.Email != "" && a.Password != "" {
			usable = append(usable, a)
		}
	}
	return &BrowserSearcher{python: python, script: script, cdnBase: cdnBase, currency: currency, accounts: usable}
}

func (s *BrowserSearcher) pick() Account {
	i := atomic.AddUint64(&s.next, 1) - 1
	return s.accounts[i%uint64(len(s.accounts))]
}

// searchArgs builds the search.py argv for an account + query (pure/testable).
func searchArgs(script, currency string, a Account, p ProductParams, keyword string) []string {
	pages := p.PageNo
	if pages <= 0 {
		pages = 1
	}
	pageSize := p.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	args := []string{
		script,
		"--email", a.Email,
		"--password", a.Password,
		"--country", p.Country,
		"--currency", currency,
		"--query", keyword,
		"--pages", strconv.Itoa(pages),
		"--page-size", strconv.Itoa(pageSize),
	}
	if a.Proxy != "" {
		args = append(args, "--proxy", a.Proxy)
	}
	if p.StartDate != "" {
		args = append(args, "--start-date", p.StartDate)
	}
	if p.EndDate != "" {
		args = append(args, "--end-date", p.EndDate)
	}
	return args
}

// Search logs in with a rotated account and returns products matching keyword.
func (s *BrowserSearcher) Search(ctx context.Context, p ProductParams, keyword string) ([]Product, error) {
	if len(s.accounts) == 0 {
		return nil, ErrNoSearchAccounts
	}
	if keyword == "" {
		return nil, fmt.Errorf("kalodata: keyword is required for search")
	}
	acc := s.pick()
	args := searchArgs(s.script, s.currency, acc, p, keyword)

	cmd := exec.CommandContext(ctx, s.python, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	switch code {
	case searchExitOK:
		// ok
	case searchExitLoginFail:
		return nil, fmt.Errorf("kalodata: search login failed (%s): %s", acc.Email, trim(stderr.Bytes()))
	case searchExitUsage:
		return nil, fmt.Errorf("kalodata: search.py usage error: %s", trim(stderr.Bytes()))
	default:
		return nil, fmt.Errorf("kalodata: search.py exit %d: %v: %s", code, err, trim(stderr.Bytes()))
	}

	var env listEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		return nil, fmt.Errorf("kalodata: decode search output: %w", err)
	}
	return mapProducts(env.Data, s.cdnBase), nil
}
