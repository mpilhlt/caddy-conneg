package connegmatcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func newRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Origin", "http://example.com")
	req = req.WithContext(context.WithValue(req.Context(), caddyhttp.VarsCtxKey, map[string]any{}))
	return req
}

func testContext() caddy.Context {
	var ctx caddy.Context
	return ctx
}

func TestValidate(t *testing.T) {
	t.Run("requires matcher", func(t *testing.T) {
		if err := (MatchConneg{}).Validate(); err == nil {
			t.Fatal("expected validation to fail without any match criteria")
		}
	})

	t.Run("allows variables when offers exist", func(t *testing.T) {
		err := (MatchConneg{
			MatchTypes: []string{"text/html"},
			VarType:    "format",
		}).Validate()
		if err != nil {
			t.Fatalf("expected validation to succeed, got %v", err)
		}
	})
}

func TestGetWeight(t *testing.T) {
	tests := map[string]int{
		"0":     0,
		"0.5":   500,
		"0.50":  500,
		"0.500": 500,
		"1":     1000,
		"1.0":   1000,
		"1.000": 1000,
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, ok := getWeight(input)
			if !ok {
				t.Fatalf("expected %q to parse", input)
			}
			if got != want {
				t.Fatalf("expected %q to parse to %d, got %d", input, want, got)
			}
		})
	}
}

func TestMatchTypeQueryOverride(t *testing.T) {
	matcher := MatchConneg{
		MatchTypes:               []string{"application/tei+xml", "application/rdf+xml"},
		ForceTypeQueryString:     "format",
		MatchLanguages:           []string{"en"},
		ForceLanguageQueryString: "lang",
	}
	if err := matcher.Provision(testContext()); err != nil {
		t.Fatalf("provision failed: %v", err)
	}

	req := newRequest(http.MethodGet, "https://example.com/resource?format=tei&lang=en")
	if !matcher.Match(req) {
		t.Fatal("expected query string override to select an offered media type")
	}
}

func TestMatchLanguageAcceptHeader(t *testing.T) {
	matcher := MatchConneg{
		MatchLanguages: []string{"en", "de"},
	}
	if err := matcher.Provision(testContext()); err != nil {
		t.Fatalf("provision failed: %v", err)
	}

	req := newRequest(http.MethodGet, "https://example.com/resource")
	req.Header.Set("Accept-Language", "de-DE,de;q=0.8")
	if !matcher.Match(req) {
		t.Fatal("expected language negotiation to match")
	}
}

func TestMatchCharsetAcceptsQualityOne(t *testing.T) {
	matcher := MatchConneg{
		MatchCharsets: []string{"utf-8"},
	}
	if err := matcher.Provision(testContext()); err != nil {
		t.Fatalf("provision failed: %v", err)
	}

	req := newRequest(http.MethodGet, "https://example.com/resource")
	req.Header.Set("Accept-Charset", "utf-8;q=1")
	if !matcher.Match(req) {
		t.Fatal("expected charset negotiation to accept q=1")
	}
}

func TestMatchStoresSelectedVars(t *testing.T) {
	matcher := MatchConneg{
		MatchTypes:     []string{"text/html"},
		MatchLanguages: []string{"en"},
		VarType:        "format",
		VarLanguage:    "language",
	}
	if err := matcher.Provision(testContext()); err != nil {
		t.Fatalf("provision failed: %v", err)
	}

	req := newRequest(http.MethodGet, "https://example.com/resource")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "en")

	if !matcher.Match(req) {
		t.Fatal("expected request to match")
	}

	if got := caddyhttp.GetVar(req.Context(), "conneg_format"); got != "text/html" {
		t.Fatalf("expected conneg_format to be text/html, got %v", got)
	}
	if got := caddyhttp.GetVar(req.Context(), "conneg_language"); got == nil {
		t.Fatal("expected conneg_language to be set")
	}
}
