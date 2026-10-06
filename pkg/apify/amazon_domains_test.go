package apify

import "testing"

func TestAmazonDomains_EUMarkets(t *testing.T) {
	want := map[string]string{
		"NL": "amazon.nl",
		"PL": "amazon.pl",
		"SE": "amazon.se",
		"BE": "amazon.com.be",
		"TR": "amazon.com.tr",
		"DE": "amazon.de", // đã có — canh không bị xoá nhầm
	}
	for code, domain := range want {
		if got := amazonDomains[code]; got != domain {
			t.Errorf("amazonDomains[%q] = %q, want %q", code, got, domain)
		}
	}
}
