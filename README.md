# Echo Browser

Go automation for **real installed Chrome**. Echo drives the browser over the Chrome DevTools Protocol with a control plane that looks like a person using Chrome: headed by default, stock binary, persistent profile, isolated-world reads, and trusted pointer/keyboard events.

It is not a fingerprint-spoofing browser, not a Chromium fork, and not a challenge solver.

## Principles

- **Be Chrome.** Use the system Google Chrome, its GPU, fonts, locale, timezone, and TLS. Do not randomize those.
- **Do not inject stealth scripts** into the page (`navigator.webdriver = false`, fake `window.chrome`, canvas noise). Those patches are signatures.
- **Never `Runtime.enable`**, never evaluate in the page world. Reads go through `Page.createIsolatedWorld`. Clicks and typing go through `Input.dispatchMouseEvent` / `Input.dispatchKeyEvent`.
- **Headed is the stealth default.** Headless is first-class (`--headless=new` on real Chrome) with a Chrome UA (no `HeadlessChrome`) and a real `--screen-info` instead of 800×600. `Hidden` is headed Chrome parked off-screen when you need no window but still want a real display.
- **No vendor bypasses.** Cloudflare Turnstile, reCAPTCHA, Akamai, Kasada, DataDome, and similar challenges are out of scope. A headed window can wait while a human solves them.

## Install

```bash
go get github.com/Echo-Laboratories/echo-browser
```

Requires Go 1.23+ and Google Chrome.

## Usage

```go
package main

import (
    "context"
    "log"
    "time"

    echo "github.com/Echo-Laboratories/echo-browser"
    "github.com/Echo-Laboratories/echo-browser/pkg/input"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    b, err := echo.Launch(ctx, echo.Options{
        // ChromePath: "",  // auto-detect system Google Chrome
        // Headless:   false, // true = --headless=new on the same binary
        // Hidden:     false, // headed, off-screen
        // Profile:    "default",
        StartURL: "https://example.com",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer b.Close()

    page, err := b.Page(ctx)
    if err != nil {
        log.Fatal(err)
    }

    if err := page.Locator("#email").Fill(ctx, "user@email.com"); err != nil {
        log.Fatal(err)
    }
    if err := page.Locator("#password").Type(ctx, "secret", input.Typing{WPM: 180, Mistakes: true}); err != nil {
        log.Fatal(err)
    }
    if err := page.Locator("#submit").Click(ctx); err != nil {
        log.Fatal(err)
    }
}
```

### Greenlight aliases

`GreenLight` / `YellowLight` / `RedLight` still exist. `GreenLight` now returns `(*Browser, error)` instead of calling `log.Fatal`.

```go
b, err := echo.GreenLight("", false, "https://example.com")
if err != nil {
    log.Fatal(err)
}
defer b.RedLight()

page, err := b.Page(context.Background())
page.YellowLight(500)
```

## Options

| Field | Default | Notes |
|---|---|---|
| `ChromePath` | auto-detect | Or `ECHO_CHROME_PATH` |
| `Headless` | `false` | Real Chrome `--headless=new`. UA is stock Chrome (not HeadlessChrome); screen is `--screen-info` matching the machine when possible |
| `Hidden` | `false` | Headed window off-screen (`--window-position` / `--start-minimized`). Prefer this over Headless when a detector keys on headless GPU/screen |
| `Width` / `Height` | OS / 1920×1080 | `--window-size`. Headed uses Chrome's OS default unless set |
| `Profile` | `"default"` | Under `%LOCALAPPDATA%\EchoBrowser\profiles` (Windows), `~/Library/Application Support/EchoBrowser/profiles` (macOS), `~/.local/share/echobrowser/profiles` (Linux) |
| `UserDataDir` | derived from `Profile` | Explicit Chrome user-data-dir |
| `Ephemeral` | `false` | Temp profile, deleted on `Close` |
| `Proxy` | none | Passed to Chrome as `--proxy-server` so TLS stays Chrome's. `http://user:pass@host:port` is rewritten through a local CONNECT forwarder |
| `StartURL` | `about:blank` | Navigated after attach |
| `ExtraArgs` | none | Extra Chrome flags. Launch fails if they include `--enable-automation`, `--headless` (unless `Headless` is set), `--remote-allow-origins=*`, or `--remote-debugging-port=9222` |

Do not run two Chromes against the same persistent profile at once.

## What the driver does not send

Denied by default (see `pkg/stealth`):

- `Runtime.enable`
- `Runtime.evaluate` without an isolated `contextId`
- `Page.addScriptToEvaluateOnNewDocument`
- `Network.enable`, `Fetch.enable`, request interception
- `DOM.enable`, `Overlay.*`, `Emulation.set*Override`
- `Input.insertText` of whole strings (keys are `dispatchKeyEvent`)

## Testing

```bash
go test ./...
```

Chrome end-to-end:

```bash
set ECHO_E2E=1
go test -count=1 -run "TestTrustedClickAndType|TestHeadlessFingerprint"
```

Live detector pages (rebrowser, sannysoft, …):

```bash
go run ./cmd/probe
go run ./cmd/probe -headless
go run ./cmd/probe -hidden
set ECHO_E2E_DETECT=1
go test -count=1 -run TestDetectRebrowser
```

Echo does not inject stealth scripts and does not use ChromeDriver (no `cdc_` / `$cdc_` leaks). The probe should look like this machine's Chrome: no `Runtime.enable` leak, no `navigator.webdriver`, no Playwright/Selenium bindings.

Debug:

- `ECHO_CDP_DEBUG=1` logs CDP method names
- `ECHO_CHROME_DEBUG=1` shows Chrome stdout/stderr

## Layout

```
pkg/cdp       websocket session, one reader, flattened targets
pkg/chrome    find / launch / profiles / proxy
pkg/stealth   CDP allow-list and launch-flag audit
pkg/input     Bezier mouse, Fitts timing, real key events
pkg/page      isolated-world locators
pkg/browser   Target.attach + public Browser
```

## License

MIT. Forked from [greenlight](https://github.com/bosniankicks/greenlight).
