// Package echo is the public entry point for Echo Browser.
package echo

import (
	"context"

	"github.com/Echo-Laboratories/echo-browser/pkg/browser"
	"github.com/Echo-Laboratories/echo-browser/pkg/input"
	"github.com/Echo-Laboratories/echo-browser/pkg/page"
)

type (
	Browser = browser.Browser
	Options = browser.Options
	Page    = page.Page
	Locator = page.Locator
	Typing  = input.Typing
)

// Launch starts system Chrome and attaches over CDP.
func Launch(ctx context.Context, opts Options) (*Browser, error) {
	return browser.Launch(ctx, opts)
}

// GreenLight is the Greenlight-compatible launcher. It returns an error
// instead of calling log.Fatal.
func GreenLight(execPath string, isHeadless bool, startURL string) (*Browser, error) {
	return browser.GreenLight(execPath, isHeadless, startURL)
}
