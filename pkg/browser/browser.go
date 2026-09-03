package browser

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Echo-Laboratories/echo-browser/pkg/cdp"
	"github.com/Echo-Laboratories/echo-browser/pkg/chrome"
	"github.com/Echo-Laboratories/echo-browser/pkg/page"
	"github.com/Echo-Laboratories/echo-browser/pkg/stealth"
)

// Options configure Launch.
type Options struct {
	ChromePath string
	// Headless uses Chrome's new headless mode (still the real Chrome binary).
	Headless bool
	// Hidden keeps a headed window off-screen. Ignored when Headless is set.
	// Prefer this over Headless when a detector keys on headless GPU/viewport.
	Hidden bool
	// Width and Height set --window-size. Headless defaults to 1920x1080.
	// Headed uses Chrome's OS default unless these are set.
	Width  int
	Height int
	// Profile is a persistent profile name under the EchoBrowser data dir.
	// Ignored when Ephemeral is true. Default "default".
	Profile string
	// UserDataDir overrides Profile with an explicit Chrome user-data-dir.
	UserDataDir string
	Ephemeral   bool
	Proxy       string
	StartURL    string
	ExtraArgs   []string
	// PermissiveCDP disables the stealth allowlist. Debugging only.
	PermissiveCDP bool
}

// Browser is a running Chrome plus a browser-level CDP connection.
type Browser struct {
	conn      *cdp.Conn
	proc      *chrome.Process
	policy    *stealth.Policy
	worldName string

	mu       sync.Mutex
	pages    map[string]*page.Page // sessionID
	byTarget map[string]string     // targetID -> sessionID
	main     *page.Page
	closed   bool

	ephemeralDir string
	proxyClose   interface{ Close() error }
	chromeUA     string
	chromeVer    string
}

// Launch starts system Chrome and attaches over CDP.
func Launch(ctx context.Context, opts Options) (*Browser, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	bin := opts.ChromePath
	if bin == "" {
		var err error
		bin, err = chrome.Find()
		if err != nil {
			return nil, err
		}
	}

	userDir := opts.UserDataDir
	ephemeral := opts.Ephemeral
	if userDir == "" {
		var err error
		userDir, err = chrome.ResolveUserDataDir(opts.Profile, ephemeral)
		if err != nil {
			return nil, err
		}
		if ephemeral {
			// Resolve already created a temp dir.
		}
	} else if ephemeral {
		// caller-supplied dir still deleted on Close if Ephemeral.
	}

	proxy, err := chrome.ResolveProxy(opts.Proxy)
	if err != nil {
		return nil, err
	}

	start := chrome.StartConfig{
		Binary:      bin,
		UserDataDir: userDir,
		Ephemeral:   ephemeral,
		Headless:    opts.Headless,
		Hidden:      opts.Hidden,
		Width:       opts.Width,
		Height:      opts.Height,
		ProxyServer: proxy.ChromeArg,
		ExtraArgs:   opts.ExtraArgs,
		StartURL:    "about:blank",
	}
	if opts.Headless {
		headlessGeometry(&start, bin)
	}

	proc, err := chrome.Start(ctx, start)
	if err != nil {
		if proxy.Closer != nil {
			_ = proxy.Closer.Close()
		}
		if ephemeral {
			_ = os.RemoveAll(userDir)
		}
		return nil, err
	}
	proc.ProxyClose = proxy.Closer

	ver, err := proc.Version(ctx)
	if err != nil {
		_ = proc.Kill()
		if proxy.Closer != nil {
			_ = proxy.Closer.Close()
		}
		return nil, err
	}

	conn, err := cdp.Dial(ver.WebSocketDebuggerURL, proc.AllowOrigin)
	if err != nil {
		_ = proc.Kill()
		if proxy.Closer != nil {
			_ = proxy.Closer.Close()
		}
		return nil, err
	}

	world, err := randomWorldName()
	if err != nil {
		conn.Close()
		_ = proc.Kill()
		return nil, err
	}

	b := &Browser{
		conn:         conn,
		proc:         proc,
		policy:       &stealth.Policy{Permissive: opts.PermissiveCDP},
		worldName:    world,
		pages:        make(map[string]*page.Page),
		byTarget:     make(map[string]string),
		ephemeralDir: "",
		proxyClose:   proxy.Closer,
		chromeUA:     start.UserAgent,
	}
	if opts.Headless {
		if ver, err := chrome.ProductVersion(bin); err == nil {
			b.chromeVer = ver
		}
	}
	if ephemeral {
		b.ephemeralDir = userDir
	}

	if err := b.initTargets(ctx); err != nil {
		_ = b.Close()
		return nil, err
	}

	if opts.StartURL != "" && opts.StartURL != "about:blank" {
		p, err := b.Page(ctx)
		if err != nil {
			_ = b.Close()
			return nil, err
		}
		if err := p.Goto(ctx, opts.StartURL); err != nil {
			_ = b.Close()
			return nil, err
		}
	}
	return b, nil
}

func headlessGeometry(start *chrome.StartConfig, bin string) {
	if start.UserAgent == "" {
		ver, err := chrome.ProductVersion(bin)
		if err == nil {
			start.UserAgent = chrome.DesktopUA(ver)
		}
	}
	sw, sh := chrome.DisplaySize()
	if sw <= 0 || sh <= 0 {
		sw, sh = 1920, 1080
	}
	if start.Width <= 0 {
		start.Width = sw
		if start.Width > 1920 {
			start.Width = 1920
		}
	}
	if start.Height <= 0 {
		start.Height = sh
		if start.Height > 1080 {
			start.Height = 1080
		}
	}
	if start.ScreenInfo == "" {
		start.ScreenInfo = fmt.Sprintf("{%dx%d}", sw, sh)
	}
}

func randomWorldName() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "echo_" + hex.EncodeToString(buf[:]), nil
}

func (b *Browser) browserCall(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if err := b.policy.Allow(method, params); err != nil {
		return nil, err
	}
	return b.conn.Call(ctx, "", method, params)
}

func (b *Browser) initTargets(ctx context.Context) error {
	b.conn.On("", "Target.attachedToTarget", b.onAttached)
	b.conn.On("", "Target.detachedFromTarget", b.onDetached)

	if _, err := b.browserCall(ctx, "Target.setAutoAttach", map[string]any{
		"autoAttach":             true,
		"waitForDebuggerOnStart": false,
		"flatten":                true,
	}); err != nil {
		return err
	}
	if _, err := b.browserCall(ctx, "Target.setDiscoverTargets", map[string]any{
		"discover": true,
	}); err != nil {
		return err
	}

	raw, err := b.browserCall(ctx, "Target.getTargets", nil)
	if err != nil {
		return err
	}
	var res struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
		} `json:"targetInfos"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return fmt.Errorf("browser: getTargets: %w", err)
	}
	for _, t := range res.TargetInfos {
		if t.Type != "page" {
			continue
		}
		b.mu.Lock()
		_, known := b.byTarget[t.TargetID]
		b.mu.Unlock()
		if known {
			continue
		}
		raw, err := b.browserCall(ctx, "Target.attachToTarget", map[string]any{
			"targetId": t.TargetID,
			"flatten":  true,
		})
		if err != nil {
			return err
		}
		var att struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(raw, &att); err == nil && att.SessionID != "" {
			b.addPage(t.TargetID, att.SessionID)
		}
	}

	_, err = b.Page(ctx)
	return err
}

func (b *Browser) addPage(targetID, sessionID string) *page.Page {
	if sessionID == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if p, ok := b.pages[sessionID]; ok {
		if targetID != "" {
			b.byTarget[targetID] = sessionID
		}
		if b.main == nil {
			b.main = p
		}
		return p
	}
	sess := &session{conn: b.conn, sessionID: sessionID, policy: b.policy}
	pg := page.New(sess, b.worldName)
	if b.chromeUA != "" && b.chromeVer != "" {
		pg.SetIdentity(b.chromeUA, b.chromeVer)
	}
	b.pages[sessionID] = pg
	if targetID != "" {
		b.byTarget[targetID] = sessionID
	}
	if b.main == nil {
		b.main = pg
	}
	return pg
}

func (b *Browser) onAttached(params json.RawMessage) {
	var ev struct {
		SessionID  string `json:"sessionId"`
		TargetInfo struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
		} `json:"targetInfo"`
	}
	if err := json.Unmarshal(params, &ev); err != nil {
		return
	}
	if ev.TargetInfo.Type != "page" || ev.SessionID == "" {
		return
	}
	b.addPage(ev.TargetInfo.TargetID, ev.SessionID)
}

func (b *Browser) onDetached(params json.RawMessage) {
	var ev struct {
		SessionID string `json:"sessionId"`
		TargetID  string `json:"targetId"`
	}
	_ = json.Unmarshal(params, &ev)
	b.mu.Lock()
	defer b.mu.Unlock()
	if p, ok := b.pages[ev.SessionID]; ok {
		if p == b.main {
			b.main = nil
		}
		delete(b.pages, ev.SessionID)
	}
	if ev.TargetID != "" {
		delete(b.byTarget, ev.TargetID)
	}
}

func withDefaultTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

// Page returns the initial tab, waiting until it is attached.
func (b *Browser) Page(ctx context.Context) (*page.Page, error) {
	ctx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		b.mu.Lock()
		p := b.main
		b.mu.Unlock()
		if p != nil {
			return p, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("browser: waiting for page: %w", ctx.Err())
		case <-tick.C:
		case <-b.conn.Closed():
			return nil, cdp.ErrClosed
		}
	}
}

// NewPage opens a new tab.
func (b *Browser) NewPage(ctx context.Context) (*page.Page, error) {
	raw, err := b.browserCall(ctx, "Target.createTarget", map[string]any{"url": "about:blank"})
	if err != nil {
		return nil, err
	}
	var res struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		b.mu.Lock()
		if sid, ok := b.byTarget[res.TargetID]; ok {
			p := b.pages[sid]
			b.mu.Unlock()
			if p != nil {
				return p, nil
			}
		} else {
			b.mu.Unlock()
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("browser: waiting for new page: %w", ctx.Err())
		case <-tick.C:
		case <-b.conn.Closed():
			return nil, cdp.ErrClosed
		}
	}
}

// Pages returns currently attached page targets.
func (b *Browser) Pages() []*page.Page {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]*page.Page, 0, len(b.pages))
	for _, p := range b.pages {
		out = append(out, p)
	}
	return out
}

// Close quits Chrome. Persistent profiles are kept; ephemeral dirs are removed.
func (b *Browser) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = b.browserCall(ctx, "Browser.close", nil)
	_ = b.conn.Close()
	_ = b.proc.Kill()
	if b.proxyClose != nil {
		_ = b.proxyClose.Close()
	}
	if b.ephemeralDir != "" {
		_ = os.RemoveAll(b.ephemeralDir)
	}
	return nil
}

// RedLight is Close (Greenlight alias).
func (b *Browser) RedLight() error { return b.Close() }

// WorldName is the isolated-world name for this browser instance (tests).
func (b *Browser) WorldName() string { return b.worldName }
