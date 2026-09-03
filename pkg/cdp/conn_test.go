package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func testServer(t *testing.T, handle func(ws *websocket.Conn)) (wsURL string, origin string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		handle(ws)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), srv.URL
}

func TestCallRoundTrip(t *testing.T) {
	url, origin := testServer(t, func(ws *websocket.Conn) {
		for {
			var msg map[string]any
			if err := ws.ReadJSON(&msg); err != nil {
				return
			}
			_ = ws.WriteJSON(map[string]any{
				"id":     msg["id"],
				"result": map[string]any{"ok": true, "method": msg["method"]},
			})
		}
	})
	conn, err := Dial(url, origin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	raw, err := conn.Call(context.Background(), "", "Target.getTargets", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		OK     bool   `json:"ok"`
		Method string `json:"method"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Method != "Target.getTargets" {
		t.Fatalf("got %+v", got)
	}
}

func TestSessionIDIsForwarded(t *testing.T) {
	url, origin := testServer(t, func(ws *websocket.Conn) {
		var msg map[string]any
		if err := ws.ReadJSON(&msg); err != nil {
			return
		}
		if msg["sessionId"] != "sess-1" {
			_ = ws.WriteJSON(map[string]any{
				"id":    msg["id"],
				"error": map[string]any{"code": -1, "message": "missing session"},
			})
			return
		}
		_ = ws.WriteJSON(map[string]any{"id": msg["id"], "result": map[string]any{}})
	})
	conn, err := Dial(url, origin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.Call(context.Background(), "sess-1", "Page.enable", nil); err != nil {
		t.Fatal(err)
	}
}

func TestProtocolError(t *testing.T) {
	url, origin := testServer(t, func(ws *websocket.Conn) {
		var msg map[string]any
		_ = ws.ReadJSON(&msg)
		_ = ws.WriteJSON(map[string]any{
			"id":    msg["id"],
			"error": map[string]any{"code": -32000, "message": "Cannot find context"},
		})
	})
	conn, err := Dial(url, origin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, err = conn.Call(context.Background(), "", "Runtime.evaluate", nil)
	if err == nil {
		t.Fatal("expected protocol error")
	}
	var pe *Error
	if !errors.As(err, &pe) || pe.Code != -32000 {
		t.Fatalf("err = %v", err)
	}
}

func TestEventDispatch(t *testing.T) {
	started := make(chan struct{})
	url, origin := testServer(t, func(ws *websocket.Conn) {
		close(started)
		_ = ws.WriteJSON(map[string]any{
			"method":    "Page.lifecycleEvent",
			"sessionId": "s1",
			"params":    map[string]any{"name": "load", "frameId": "f"},
		})
		// keep open until client disconnects
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	})
	conn, err := Dial(url, origin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	got := make(chan string, 1)
	unsub := conn.On("s1", "Page.lifecycleEvent", func(params json.RawMessage) {
		var p struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(params, &p)
		got <- p.Name
	})
	defer unsub()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start")
	}
	select {
	case name := <-got:
		if name != "load" {
			t.Fatalf("name = %s", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event not received")
	}
}

func TestEmptySessionListenerIgnoresPageEvents(t *testing.T) {
	url, origin := testServer(t, func(ws *websocket.Conn) {
		_ = ws.WriteJSON(map[string]any{
			"method":    "Page.lifecycleEvent",
			"sessionId": "page-sess",
			"params":    map[string]any{"name": "load"},
		})
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	})
	conn, err := Dial(url, origin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	got := make(chan struct{}, 1)
	unsub := conn.On("", "Page.lifecycleEvent", func(params json.RawMessage) {
		got <- struct{}{}
	})
	defer unsub()

	select {
	case <-got:
		t.Fatal("browser-session listener received a page-session event")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestCallCancelled(t *testing.T) {
	url, origin := testServer(t, func(ws *websocket.Conn) {
		var msg map[string]any
		_ = ws.ReadJSON(&msg)
		time.Sleep(500 * time.Millisecond)
		_ = ws.WriteJSON(map[string]any{"id": msg["id"], "result": map[string]any{}})
	})
	conn, err := Dial(url, origin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = conn.Call(ctx, "", "Page.enable", nil)
	if err == nil {
		t.Fatal("expected timeout")
	}
}

func TestPendingUnblockedOnClose(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	url, origin := testServer(t, func(ws *websocket.Conn) {
		wg.Done()
		var msg map[string]any
		_ = ws.ReadJSON(&msg)
		// never reply
		time.Sleep(2 * time.Second)
	})
	conn, err := Dial(url, origin)
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	done := make(chan error, 1)
	go func() {
		_, err := conn.Call(context.Background(), "", "Page.enable", nil)
		done <- err
	}()
	time.Sleep(30 * time.Millisecond)
	conn.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call was not unblocked")
	}
}
