package crawler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

var (
	ErrChallenged = errors.New("crawler: datadome challenge blocked after solver retries")
	ErrEmpty      = errors.New("crawler: listing page returned zero product cards")
)

const (
	exitOK         = 0
	exitChallenged = 2
	exitEmpty      = 3
	exitUsage      = 4
)

type Offer struct {
	Position          int               `json:"position"`
	ProductID         *string           `json:"product_id"`
	OfferID           *string           `json:"offer_id"`
	Title             *string           `json:"title"`
	Brand             *string           `json:"brand"`
	Model             *string           `json:"model"`
	Condition         *string           `json:"condition"`
	Seller            *string           `json:"seller"`
	URL               *string           `json:"url"`
	ImageURL          *string           `json:"image_url"`
	Price             *float64          `json:"price"`
	PriceWithDelivery *float64          `json:"price_with_delivery"`
	Currency          *string           `json:"currency"`
	Rating            *float64          `json:"rating"`
	ReviewCount       *int              `json:"review_count"`
	BoughtRecently    *int              `json:"bought_recently"`
	SuperSeller       bool              `json:"super_seller"`
	Smart             bool              `json:"smart"`
	Delivery          *string           `json:"delivery"`
	Sponsored         bool              `json:"sponsored"`
	Page              int               `json:"page"`
	Specs             map[string]string `json:"specs"`
}

type Result struct {
	URL    string  `json:"url"`
	Bytes  int     `json:"bytes"`
	Offers []Offer `json:"offers"`
}

type FetchParams struct {
	URL      string
	Keyword  string
	Pages    int
	MaxItems int
}

type Client interface {
	Fetch(ctx context.Context, params FetchParams) (*Result, error)
}

type Config struct {
	Python     string
	Script     string
	ProfileDir string
	Proxy      string
	Xvfb       bool
}

type Subprocess struct {
	Cfg Config
}

func NewSubprocess(cfg Config) *Subprocess {
	return &Subprocess{Cfg: cfg}
}

func (s *Subprocess) Fetch(ctx context.Context, params FetchParams) (*Result, error) {
	name, args := s.command(params)

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}

	switch code {
	case exitOK:
		// ok
	case exitChallenged:
		return nil, fmt.Errorf("%w: %s", ErrChallenged, trimErr(stderr.String()))
	case exitEmpty:
		return nil, fmt.Errorf("%w: %s", ErrEmpty, trimErr(stderr.String()))
	case exitUsage:
		return nil, fmt.Errorf("crawler: fetch.py usage error: %s", trimErr(stderr.String()))
	default:
		return nil, fmt.Errorf("crawler: fetch.py exit %d: %v: %s", code, err, trimErr(stderr.String()))
	}

	var res Result
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return nil, fmt.Errorf("crawler: decode fetch.py output: %w", err)
	}
	return &res, nil
}

func (s *Subprocess) command(params FetchParams) (string, []string) {
	args := []string{s.Cfg.Script}
	if params.URL != "" {
		args = append(args, params.URL)
	}
	if params.Keyword != "" {
		args = append(args, "--keyword", params.Keyword)
	}
	if params.Pages > 0 {
		args = append(args, "--pages", strconv.Itoa(params.Pages))
	}
	if params.MaxItems > 0 {
		args = append(args, "--max-items", strconv.Itoa(params.MaxItems))
	}
	if s.Cfg.ProfileDir != "" && !strings.Contains(s.Cfg.ProfileDir, "${") {
		args = append(args, "--profile", s.Cfg.ProfileDir)
	}
	if s.Cfg.Proxy != "" && !strings.Contains(s.Cfg.Proxy, "${") {
		args = append(args, "--proxy", s.Cfg.Proxy)
	}

	argv := append([]string{s.Cfg.Python}, args...)
	if s.Cfg.Xvfb {
		return "xvfb-run", append([]string{"-a"}, argv...)
	}
	return argv[0], argv[1:]
}

func trimErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 600 {
		return s[:600] + "..."
	}
	return s
}
