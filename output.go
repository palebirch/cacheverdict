package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

func writeJSON(w io.Writer, v Verdict) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printHuman(w io.Writer, source string, status int, v Verdict) {
	fmt.Fprintf(w, "%s (status %d)\n\n", source, status)

	switch {
	case !v.Cacheable:
		fmt.Fprintln(w, "verdict: NOT CACHEABLE")
	case v.SharedCacheable:
		fmt.Fprintln(w, "verdict: CACHEABLE by shared and private caches")
	default:
		fmt.Fprintln(w, "verdict: CACHEABLE by private caches only")
	}

	if v.FreshSeconds != nil {
		fmt.Fprintf(w, "fresh for: %s\n", formatDuration(*v.FreshSeconds))
	} else {
		fmt.Fprintln(w, "fresh for: unknown / heuristic")
	}

	if v.MustRevalidate {
		fmt.Fprintln(w, "revalidation: required before reuse")
	}

	if len(v.Validators) > 0 {
		fmt.Fprintf(w, "validators: %s\n", strings.Join(v.Validators, ", "))
	} else {
		fmt.Fprintln(w, "validators: none (no ETag or Last-Modified)")
	}

	if len(v.Vary) > 0 {
		fmt.Fprintf(w, "varies on: %s\n", strings.Join(v.Vary, ", "))
	}

	if len(v.Directives) > 0 {
		keys := make([]string, 0, len(v.Directives))
		for k := range v.Directives {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintln(w, "\ncache-control directives:")
		for _, k := range keys {
			if val := v.Directives[k]; val != "" {
				fmt.Fprintf(w, "  %s=%s\n", k, val)
			} else {
				fmt.Fprintf(w, "  %s\n", k)
			}
		}
	}

	if len(v.Reasons) > 0 {
		fmt.Fprintln(w, "\nwhy:")
		for _, r := range v.Reasons {
			fmt.Fprintf(w, "  - %s\n", r)
		}
	}
}

func formatDuration(seconds int64) string {
	if seconds < 0 {
		return "already expired"
	}
	return (time.Duration(seconds) * time.Second).String()
}
