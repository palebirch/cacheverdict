# cacheverdict

Caching headers are simple in isolation and confusing in combination.
`Cache-Control: private, max-age=3600` next to a `Vary: Cookie` and an
`Expires` header from five years ago is a normal thing to find in the wild,
and answering "so will this actually get cached, and by what" from memory
gets things wrong more often than it should.

`cacheverdict` answers exactly that question for one response: given its
headers, will a cache store it, which kind of cache (private/browser vs.
shared/CDN), and for how long before it needs revalidation.

It does not audit your whole site, diff two responses, or check any header
other than the caching ones. One question, answered well.

## Usage

Point it at a URL and it fetches the headers for you:

```
$ cacheverdict https://example.com/
https://example.com/ (status 200)

verdict: CACHEABLE by shared and private caches
fresh for: 21m40s
validators: ETag
varies on: Accept-Encoding

cache-control directives:
  max-age=1300

why:
  - freshness computed from Expires (no max-age present)
```

Or feed it a raw header dump if you already have one (from `curl -sI`, a
browser devtools export, a log line, whatever):

```
$ curl -sI https://example.com/ > headers.txt
$ cacheverdict --file headers.txt
```

Add `--json` for scripting:

```
$ cacheverdict --json https://example.com/
{
  "cacheable": true,
  "shared_cacheable": true,
  "private_cacheable": true,
  "fresh_seconds": 1300,
  "must_revalidate": false,
  "validators": ["ETag"],
  "vary": ["Accept-Encoding"],
  "directives": {"max-age": "1300"},
  "reasons": ["..."]
}
```

## Flags

```
--json          output the JSON shape above instead of the human summary
--method GET    HTTP method to use when fetching a URL (default GET)
--file path     read raw response headers from a file instead of a request
```

## What it actually checks

`Cache-Control` (`no-store`, `no-cache`, `private`, `public`, `max-age`,
`s-maxage`, `must-revalidate`), `Expires` as a fallback when there's no
`max-age`, `Vary`, and the presence of `ETag` / `Last-Modified` as
revalidation validators. `stale-while-revalidate` and `stale-if-error` are
parsed into the directives map but not yet factored into the verdict itself.

## Install

```
go install .
```

or just `go build` in a checkout.
