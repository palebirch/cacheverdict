package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	var (
		jsonOut bool
		method  string
		file    string
	)
	flag.BoolVar(&jsonOut, "json", false, "output machine-readable JSON instead of a human summary")
	flag.StringVar(&method, "method", "GET", "HTTP method to use when fetching a URL")
	flag.StringVar(&file, "file", "", "read raw response headers from a file instead of making a request")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "cacheverdict answers one question: will this HTTP response be cached, by what, and for how long?\n\n")
		fmt.Fprintf(os.Stderr, "usage:\n  cacheverdict [flags] <url>\n  cacheverdict [flags] --file headers.txt\n\nflags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	var (
		headers http.Header
		status  int
		source  string
		err     error
	)

	switch {
	case file != "":
		headers, status, err = readHeadersFromFile(file)
		source = file
	case flag.NArg() == 1:
		url := flag.Arg(0)
		headers, status, err = fetchHeaders(method, url)
		source = url
	default:
		flag.Usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "cacheverdict: %v\n", err)
		os.Exit(1)
	}

	v := Evaluate(headers, status)

	if jsonOut {
		if err := writeJSON(os.Stdout, v); err != nil {
			fmt.Fprintf(os.Stderr, "cacheverdict: %v\n", err)
			os.Exit(1)
		}
		return
	}

	printHuman(os.Stdout, source, status, v)
}

func fetchHeaders(method, url string) (http.Header, int, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("building request: %w", err)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.Header, resp.StatusCode, nil
}

// readHeadersFromFile parses a raw HTTP header dump, e.g. what you'd get
// from `curl -sI` piped to a file. The optional status line comes first,
// then one "Name: value" pair per line.
func readHeadersFromFile(path string) (http.Header, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	headers := make(http.Header)
	status := 0
	first := true

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if first {
			first = false
			if strings.HasPrefix(line, "HTTP/") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if code, err := strconv.Atoi(fields[1]); err == nil {
						status = code
					}
				}
				continue
			}
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		headers.Add(strings.TrimSpace(name), strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}
	return headers, status, nil
}
