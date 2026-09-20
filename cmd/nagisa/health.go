package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// healthFlag turns the binary into its own health probe:
//
//	nagisa -health http://127.0.0.1:8000/v1/system/health
//
// It performs a single GET, returns exit code 0 when the service answers 200
// with `"status":"ok"`, and 1 otherwise, which is exactly the contract a
// container healthcheck needs. It exists because the runtime image is
// distroless: there is no shell, no wget and no curl inside it, so the check
// has to be something the image already contains.
//
// It never starts the server and never touches the database, so it is safe to
// run alongside the real process.
var healthFlag = flag.String(
	"health",
	"",
	"probe a running instance over HTTP and exit, eg http://127.0.0.1:8000/v1/system/health",
)

// healthProbe runs the probe described on healthFlag. It reports whether a
// probe was requested: false means the caller should carry on and boot the
// server.
func healthProbe(target string) (bool, error) {
	if strings.TrimSpace(target) == "" {
		return false, nil
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(target)
	if err != nil {
		return true, fmt.Errorf("health probe %s: %w", target, err)
	}
	defer resp.Body.Close()

	// The endpoint answers 200 even when a dependency is unreachable and puts
	// the verdict in the body, so the status in the payload decides.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return true, fmt.Errorf("health probe %s: http %d", target, resp.StatusCode)
	}
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return true, fmt.Errorf("health probe %s: decode reply: %w", target, err)
	}
	if payload.Status != "ok" {
		return true, fmt.Errorf("health probe %s: status %q", target, payload.Status)
	}
	return true, nil
}

// runHealthProbe is the main-package entry point for the -health flag.
func runHealthProbe(target string) bool {
	probed, err := healthProbe(target)
	if !probed {
		return false
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "nagisa: %v\n", err)
		os.Exit(1)
	}
	return true
}
