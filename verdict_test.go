package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func hdr(pairs ...string) http.Header {
	h := make(http.Header)
	for i := 0; i+1 < len(pairs); i += 2 {
		h.Add(pairs[i], pairs[i+1])
	}
	return h
}

func hasReason(v Verdict, sub string) bool {
	for _, r := range v.Reasons {
		if strings.Contains(r, sub) {
			return true
		}
	}
	return false
}

func TestEvaluateStorageDirectives(t *testing.T) {
	tests := []struct {
		name        string
		cc          string
		wantAny     bool
		wantShared  bool
		wantPrivate bool
	}{
		{"no-store", "no-store", false, false, false},
		{"no-store beats max-age", "max-age=600, no-store", false, false, false},
		{"no-store beats public", "public, no-store", false, false, false},
		{"private", "private, max-age=60", true, false, true},
		{"public", "public, max-age=60", true, true, true},
		{"no directives", "", true, true, true},
		{"uppercase directive", "NO-STORE", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := hdr()
			if tt.cc != "" {
				h.Set("Cache-Control", tt.cc)
			}
			v := Evaluate(h, 200)
			if v.Cacheable != tt.wantAny || v.SharedCacheable != tt.wantShared || v.PrivateCacheable != tt.wantPrivate {
				t.Errorf("got cacheable=%v shared=%v private=%v, want %v %v %v",
					v.Cacheable, v.SharedCacheable, v.PrivateCacheable,
					tt.wantAny, tt.wantShared, tt.wantPrivate)
			}
			if !tt.wantAny && v.FreshSeconds != nil {
				t.Errorf("uncacheable response should have no freshness, got %d", *v.FreshSeconds)
			}
		})
	}
}

func TestEvaluateFreshness(t *testing.T) {
	tests := []struct {
		name string
		cc   string
		want int64
	}{
		{"max-age only", "max-age=300", 300},
		{"s-maxage overrides max-age for shared", "max-age=60, s-maxage=900", 900},
		{"s-maxage alone", "s-maxage=120", 120},
		{"public with s-maxage", "public, max-age=10, s-maxage=20", 20},
		{"max-age zero", "max-age=0", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Evaluate(hdr("Cache-Control", tt.cc), 200)
			if v.FreshSeconds == nil {
				t.Fatalf("no freshness, want %d", tt.want)
			}
			if *v.FreshSeconds != tt.want {
				t.Errorf("fresh = %d, want %d", *v.FreshSeconds, tt.want)
			}
		})
	}
}

func TestEvaluatePrivateIgnoresSMaxAge(t *testing.T) {
	v := Evaluate(hdr("Cache-Control", "private, max-age=60, s-maxage=900"), 200)
	if v.FreshSeconds == nil || *v.FreshSeconds != 60 {
		t.Fatalf("private response should use max-age=60, got %v", v.FreshSeconds)
	}
	if hasReason(v, "s-maxage") {
		t.Errorf("s-maxage should not be reported for a private response: %v", v.Reasons)
	}
}

func TestEvaluateExpiresFallback(t *testing.T) {
	exp := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)

	v := Evaluate(hdr("Expires", exp), 200)
	if v.FreshSeconds == nil {
		t.Fatal("expected freshness from Expires")
	}
	if *v.FreshSeconds < 3590 || *v.FreshSeconds > 3600 {
		t.Errorf("fresh = %d, want about 3600", *v.FreshSeconds)
	}

	v = Evaluate(hdr("Cache-Control", "max-age=30", "Expires", exp), 200)
	if v.FreshSeconds == nil || *v.FreshSeconds != 30 {
		t.Errorf("max-age should win over Expires, got %v", v.FreshSeconds)
	}
}

func TestEvaluateRevalidation(t *testing.T) {
	v := Evaluate(hdr("Cache-Control", "no-cache"), 200)
	if !v.Cacheable || !v.MustRevalidate {
		t.Errorf("no-cache should be storable and force revalidation: %+v", v)
	}
	if !hasReason(v, "no-cache") {
		t.Errorf("missing no-cache reason: %v", v.Reasons)
	}

	v = Evaluate(hdr("Cache-Control", "max-age=60, must-revalidate"), 200)
	if !v.MustRevalidate || !hasReason(v, "must-revalidate") {
		t.Errorf("must-revalidate not reported: %+v", v)
	}

	v = Evaluate(hdr("Cache-Control", "max-age=60"), 200)
	if v.MustRevalidate {
		t.Error("MustRevalidate set without a directive asking for it")
	}
}

func TestEvaluateValidatorsAndVary(t *testing.T) {
	v := Evaluate(hdr(
		"ETag", `"abc"`,
		"Last-Modified", "Wed, 01 Jan 2020 00:00:00 GMT",
		"Vary", "Accept-Encoding, Cookie",
	), 200)
	if len(v.Validators) != 2 || v.Validators[0] != "ETag" || v.Validators[1] != "Last-Modified" {
		t.Errorf("validators = %v", v.Validators)
	}
	if len(v.Vary) != 2 || v.Vary[0] != "Accept-Encoding" || v.Vary[1] != "Cookie" {
		t.Errorf("vary = %v", v.Vary)
	}
	if !hasReason(v, "heuristic") {
		t.Errorf("validators without freshness should mention heuristics: %v", v.Reasons)
	}
}

func TestEvaluateNoSignals(t *testing.T) {
	v := Evaluate(hdr(), 200)
	if v.FreshSeconds != nil {
		t.Errorf("unexpected freshness %d", *v.FreshSeconds)
	}
	if !hasReason(v, "little basis") {
		t.Errorf("expected a no-signals reason: %v", v.Reasons)
	}
}

func TestEvaluateNon200(t *testing.T) {
	v := Evaluate(hdr("Cache-Control", "max-age=60"), 404)
	if !hasReason(v, "status 404") {
		t.Errorf("expected status note: %v", v.Reasons)
	}
	v = Evaluate(hdr("Cache-Control", "max-age=60"), 200)
	if hasReason(v, "status") {
		t.Errorf("200 should not produce a status note: %v", v.Reasons)
	}
}

func TestParseCacheControl(t *testing.T) {
	cc := parseCacheControl(` Max-Age=60 , private,, community="UCI" `)
	if cc["max-age"] != "60" {
		t.Errorf("max-age = %q", cc["max-age"])
	}
	if _, ok := cc["private"]; !ok {
		t.Error("private missing")
	}
	if cc["community"] != "UCI" {
		t.Errorf("quoted value not unquoted: %q", cc["community"])
	}
	if len(cc) != 3 {
		t.Errorf("got %d directives, want 3: %v", len(cc), cc)
	}
	if parseCacheControl("") != nil {
		t.Error("empty header should yield nil")
	}
}

func TestIntDirective(t *testing.T) {
	cc := map[string]string{"max-age": "abc", "s-maxage": "5"}
	if _, ok := intDirective(cc, "max-age"); ok {
		t.Error("non-numeric value accepted")
	}
	if n, ok := intDirective(cc, "s-maxage"); !ok || n != 5 {
		t.Errorf("s-maxage = %d, %v", n, ok)
	}
	if _, ok := intDirective(cc, "missing"); ok {
		t.Error("missing key reported present")
	}
}
