package input

import (
	"context"
	"encoding/json"
)

// Caller sends CDP methods on a page session.
type Caller interface {
	Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error)
}

func dispatch(ctx context.Context, c Caller, method string, params map[string]any) error {
	_, err := c.Call(ctx, method, params)
	return err
}
