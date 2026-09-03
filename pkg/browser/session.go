package browser

import (
	"context"
	"encoding/json"

	"github.com/Echo-Laboratories/echo-browser/pkg/cdp"
	"github.com/Echo-Laboratories/echo-browser/pkg/stealth"
)

type session struct {
	conn      *cdp.Conn
	sessionID string
	policy    *stealth.Policy
}

func (s *session) Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	if err := s.policy.Allow(method, params); err != nil {
		return nil, err
	}
	return s.conn.Call(ctx, s.sessionID, method, params)
}

func (s *session) On(method string, fn func(params json.RawMessage)) func() {
	return s.conn.On(s.sessionID, method, fn)
}
