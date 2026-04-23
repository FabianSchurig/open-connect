// `oc` — Open-Connect CLI for pipelines and operators (Epic I).
//
// Subcommands:
//   oc claim create --tags ros2-hil,x86 --count 5 --version 2.4.0 --ttl 60s
//   oc claim wait <id> --until=ready --timeout=10m
//   oc claim release <id>
//
// The CLI is a thin HTTP client; it does NOT depend on internal control-plane
// packages. This makes it usable from CI runners without bringing in the
// whole control-plane build.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		return usage()
	}
	if args[0] != "claim" {
		return usage()
	}
	switch args[1] {
	case "create":
		return cmdCreate(args[2:])
	case "wait":
		return cmdWait(args[2:])
	case "release":
		return cmdRelease(args[2:])
	case "get":
		return cmdGet(args[2:])
	default:
		return usage()
	}
}

func usage() error {
	fmt.Fprintln(os.Stderr, `usage:
  oc claim create --tags T1,T2 --count N --version V --ttl 60s [--prep 10m]
  oc claim get <id>
  oc claim wait <id> --until=ready --timeout=10m
  oc claim release <id>

env:
  OC_BASE_URL    control-plane base URL (default http://localhost:8080)
  OC_SUBJECT     RBAC subject (default $USER)`)
	return errors.New("invalid usage")
}

func cmdCreate(args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	tags := fs.String("tags", "", "comma-separated tags (required)")
	count := fs.Uint("count", 1, "number of devices")
	version := fs.String("version", "", "desired version")
	ttl := fs.Duration("ttl", 30*time.Minute, "claim TTL")
	prep := fs.Duration("prep", 5*time.Minute, "preparation timeout per lock")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tags == "" {
		return errors.New("--tags is required")
	}
	body := map[string]any{
		"count":                       *count,
		"tags":                        strings.Split(*tags, ","),
		"desired_version":             *version,
		"ttl_seconds":                 uint32(ttl.Seconds()),
		"preparation_timeout_seconds": uint32(prep.Seconds()),
	}
	resp, err := request("POST", "/v1/claims", body)
	if err != nil {
		return err
	}
	fmt.Println(string(resp))
	return nil
}

func cmdGet(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: oc claim get <id>")
	}
	out, err := request("GET", "/v1/claims/"+args[0], nil)
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func cmdWait(args []string) error {
	fs := flag.NewFlagSet("wait", flag.ContinueOnError)
	until := fs.String("until", "ready", "claim state to wait for (ready|locked)")
	timeout := fs.Duration("timeout", 10*time.Minute, "give up after this duration")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: oc claim wait <id> [flags]")
	}
	id := fs.Arg(0)
	target := titleCase(*until)
	deadline := time.Now().Add(*timeout)
	for time.Now().Before(deadline) {
		out, err := request("GET", "/v1/claims/"+id, nil)
		if err != nil {
			return err
		}
		var c struct{ State string }
		_ = json.Unmarshal(out, &c)
		if c.State == target {
			fmt.Println(string(out))
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for state=%s", target)
}

func cmdRelease(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: oc claim release <id>")
	}
	_, err := request("DELETE", "/v1/claims/"+args[0], nil)
	return err
}

// titleCase returns s with the first ASCII letter upper-cased and the rest
// lower-cased. (We only ever pass tiny state names like "ready" / "locked".)
// Avoids the Go-1.18-deprecated strings.Title.
func titleCase(s string) string {
	s = strings.ToLower(s)
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}

func request(method, path string, body any) ([]byte, error) {
	base := os.Getenv("OC_BASE_URL")
	if base == "" {
		base = "http://localhost:8080"
	}
	subject := os.Getenv("OC_SUBJECT")
	if subject == "" {
		subject = os.Getenv("USER")
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, rdr)
	if err != nil {
		return nil, err
	}
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Subject", subject)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(out)))
	}
	return out, nil
}
