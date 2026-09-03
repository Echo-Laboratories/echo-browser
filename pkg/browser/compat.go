package browser

import "context"

// GreenLight launches Chrome with the original positional arguments.
// Unlike the historical Greenlight API, it returns an error instead of calling log.Fatal.
func GreenLight(execPath string, isHeadless bool, startURL string) (*Browser, error) {
	return Launch(context.Background(), Options{
		ChromePath: execPath,
		Headless:   isHeadless,
		StartURL:   startURL,
	})
}
