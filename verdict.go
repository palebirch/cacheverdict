package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Verdict is the answer to "will this response be cached, by what, and for
// how long", derived from Cache-Control, Expires, Vary and the validator
// headers. JSON tags define the --json output shape, so treat them as a
// stable interface once this ships.
type Verdict struct {
	Cacheable        bool              `json:"cacheable"`
	SharedCacheable  bool              `json:"shared_cacheable"`
	PrivateCacheable bool              `json:"private_cacheable"`
	FreshSeconds     *int64            `json:"fresh_seconds,omitempty"`
	MustRevalidate   bool              `json:"must_revalidate"`
	Validators       []string          `json:"validators,omitempty"`
	Vary             []string          `json:"vary,omitempty"`
	Directives       map[string]string `json:"directives,omitempty"`
	Reasons          []string          `json:"reasons"`
}

func Evaluate(h http.Header, status int) Verdict {
	v := Verdict{}
	cc := parseCacheControl(h.Get("Cache-Control"))
	if len(cc) > 0 {
		v.Directives = cc
	}

	if vary := h.Get("Vary"); vary != "" {
		for _, part := range strings.Split(vary, ",") {
			v.Vary = append(v.Vary, strings.TrimSpace(part))
		}
	}
	if h.Get("ETag") != "" {
		v.Validators = append(v.Validators, "ETag")
	}
	if h.Get("Last-Modified") != "" {
		v.Validators = append(v.Validators, "Last-Modified")
	}

	_, noStore := cc["no-store"]
	_, noCache := cc["no-cache"]
	_, private := cc["private"]
	_, public := cc["public"]
	_, mustRevalidate := cc["must-revalidate"]
	v.MustRevalidate = mustRevalidate || noCache

	if noStore {
		v.Reasons = append(v.Reasons, "no-store forbids storing the response in any cache")
		return v
	}

	if status != 200 {
		v.Reasons = append(v.Reasons, fmt.Sprintf("status %d is cached less predictably than 200; check your cache's policy for this status", status))
	}

	if private {
		v.PrivateCacheable = true
		v.Cacheable = true
		v.Reasons = append(v.Reasons, "private restricts storage to a single-user cache (e.g. the browser), not shared caches like a CDN")
	} else {
		v.PrivateCacheable = true
		v.SharedCacheable = true
		v.Cacheable = true
		if public {
			v.Reasons = append(v.Reasons, "public explicitly allows shared caches to store the response even if the request looked authenticated")
		}
	}

	maxAge, hasMaxAge := intDirective(cc, "max-age")
	sMaxAge, hasSMaxAge := intDirective(cc, "s-maxage")

	switch {
	case hasSMaxAge && v.SharedCacheable:
		v.FreshSeconds = &sMaxAge
		v.Reasons = append(v.Reasons, fmt.Sprintf("s-maxage=%d overrides max-age for shared caches", sMaxAge))
	case hasMaxAge:
		v.FreshSeconds = &maxAge
	default:
		if exp := h.Get("Expires"); exp != "" {
			if t, err := http.ParseTime(exp); err == nil {
				secs := int64(time.Until(t).Seconds())
				v.FreshSeconds = &secs
				v.Reasons = append(v.Reasons, "freshness computed from Expires (no max-age present)")
			}
		} else if len(v.Validators) > 0 {
			v.Reasons = append(v.Reasons, "no explicit freshness directive; caches may fall back to heuristic freshness based on Last-Modified")
		} else {
			v.Reasons = append(v.Reasons, "no freshness or validator headers present; caches have little basis to reuse this response")
		}
	}

	if noCache {
		v.Reasons = append(v.Reasons, "no-cache allows storage but forces revalidation before every reuse")
	}
	if mustRevalidate {
		v.Reasons = append(v.Reasons, "must-revalidate forbids serving a stale copy once the freshness lifetime ends")
	}

	return v
}

func parseCacheControl(header string) map[string]string {
	if header == "" {
		return nil
	}
	directives := make(map[string]string)
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, hasValue := strings.Cut(part, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if hasValue {
			directives[key] = strings.Trim(strings.TrimSpace(value), `"`)
		} else {
			directives[key] = ""
		}
	}
	return directives
}

func intDirective(cc map[string]string, key string) (int64, bool) {
	raw, ok := cc[key]
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
