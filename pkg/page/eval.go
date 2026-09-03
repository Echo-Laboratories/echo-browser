package page

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type evalEnvelope struct {
	Result struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	} `json:"result"`
	ExceptionDetails *struct {
		Text string `json:"text"`
	} `json:"exceptionDetails"`
}

func boundExpr(selector, body string) string {
	raw, _ := json.Marshal(selector)
	return fmt.Sprintf("((sel) => { %s })(%s)", body, string(raw))
}

func (p *Page) evaluate(ctx context.Context, expression string, dest any) error {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		cid, err := p.isolatedContext(ctx)
		if err != nil {
			return err
		}
		raw, err := p.call.Call(ctx, "Runtime.evaluate", map[string]any{
			"expression":    expression,
			"contextId":     cid,
			"returnByValue": true,
			"awaitPromise":  true,
		})
		if err != nil {
			if isContextErr(err) {
				p.invalidateWorld()
				last = err
				continue
			}
			return err
		}
		var env evalEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("page: evaluate decode: %w", err)
		}
		if env.ExceptionDetails != nil {
			return fmt.Errorf("page: evaluate: %s", env.ExceptionDetails.Text)
		}
		if dest == nil {
			return nil
		}
		if len(env.Result.Value) == 0 || string(env.Result.Value) == "null" {
			return errNotFound
		}
		if err := json.Unmarshal(env.Result.Value, dest); err != nil {
			return fmt.Errorf("page: evaluate value: %w", err)
		}
		return nil
	}
	if last == nil {
		last = fmt.Errorf("page: evaluate failed")
	}
	return last
}

func isContextErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "Cannot find context") ||
		strings.Contains(s, "context was destroyed") ||
		strings.Contains(s, "Inspected target navigated or closed")
}

var errNotFound = fmt.Errorf("page: element not found")
