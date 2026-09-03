package page

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBoundExprJSONEncodesSelector(t *testing.T) {
	sel := `"); alert(1); //`
	expr := boundExpr(sel, "return sel")
	raw, err := json.Marshal(sel)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(expr, string(raw)) {
		t.Fatalf("missing json encoding in %s", expr)
	}
	if strings.Contains(expr, `("); alert`) {
		t.Fatalf("selector concatenated, not json-encoded: %s", expr)
	}
}
