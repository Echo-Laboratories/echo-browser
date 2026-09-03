package chrome

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Echo-Laboratories/echo-browser/pkg/stealth"
)

// Version is Chrome's /json/version payload.
type Version struct {
	Browser              string `json:"Browser"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// Process is a running Chrome plus the debug port we asked it to bind.
type Process struct {
	Cmd         *exec.Cmd
	Port        int
	UserDataDir string
	Ephemeral   bool
	AllowOrigin string
	ProxyClose  io.Closer
	exited      chan error
}

// StartConfig is passed to Start.
type StartConfig struct {
	Binary      string
	UserDataDir string
	Ephemeral   bool
	Headless    bool
	Hidden      bool
	Width       int
	Height      int
	UserAgent   string
	ScreenInfo  string
	ProxyServer string
	ExtraArgs   []string
	StartURL    string
}

// Start launches Chrome and waits until /json/version answers.
func Start(ctx context.Context, cfg StartConfig) (*Process, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("chrome: port: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	origin := fmt.Sprintf("http://127.0.0.1:%d", port)
	args := BuildArgs(LaunchArgs{
		Port:        port,
		UserDataDir: cfg.UserDataDir,
		Headless:    cfg.Headless,
		Hidden:      cfg.Hidden,
		Width:       cfg.Width,
		Height:      cfg.Height,
		UserAgent:   cfg.UserAgent,
		ScreenInfo:  cfg.ScreenInfo,
		ProxyServer: cfg.ProxyServer,
		Extra:       cfg.ExtraArgs,
		StartURL:    cfg.StartURL,
		AllowOrigin: origin,
	})
	if issues := stealth.AuditLaunchArgs(args, cfg.Headless); len(issues) > 0 {
		return nil, fmt.Errorf("chrome: stealth-forbidden flags: %s", strings.Join(issues, ", "))
	}

	cmd := exec.Command(cfg.Binary, args...)
	if os.Getenv("ECHO_CHROME_DEBUG") == "" {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	} else {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("chrome: start: %w", err)
	}
	p := &Process{
		Cmd:         cmd,
		Port:        port,
		UserDataDir: cfg.UserDataDir,
		Ephemeral:   cfg.Ephemeral,
		AllowOrigin: origin,
		exited:      make(chan error, 1),
	}
	go func() { p.exited <- cmd.Wait() }()

	if err := waitAlive(ctx, p); err != nil {
		_ = p.Kill()
		return nil, err
	}
	return p, nil
}

func waitAlive(ctx context.Context, p *Process) error {
	client := &http.Client{Timeout: 300 * time.Millisecond}
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", p.Port)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("chrome: waiting for debug port: %w", ctx.Err())
		case err := <-p.exited:
			if err == nil {
				err = fmt.Errorf("exit 0")
			}
			return fmt.Errorf("chrome: process exited while starting: %w", err)
		case <-ticker.C:
			resp, err := client.Get(url)
			if err != nil {
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
	}
}

// Version fetches /json/version.
func (p *Process) Version(ctx context.Context) (Version, error) {
	var v Version
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/json/version", p.Port), nil)
	if err != nil {
		return v, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return v, fmt.Errorf("chrome: json/version: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return v, fmt.Errorf("chrome: json/version status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return v, fmt.Errorf("chrome: json/version decode: %w", err)
	}
	if v.WebSocketDebuggerURL == "" {
		return v, fmt.Errorf("chrome: missing webSocketDebuggerUrl")
	}
	return v, nil
}

// Kill stops Chrome. Ephemeral profiles are removed by Browser.Close, not here.
func (p *Process) Kill() error {
	if p == nil || p.Cmd == nil || p.Cmd.Process == nil {
		return nil
	}
	_ = p.Cmd.Process.Kill()
	select {
	case <-p.exited:
	case <-time.After(3 * time.Second):
	}
	return nil
}
