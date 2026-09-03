package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// ErrClosed is returned when a call is made after the connection has died.
var ErrClosed = errors.New("cdp: connection closed")

// Error is a CDP protocol error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return "cdp: unknown error"
	}
	return fmt.Sprintf("cdp: %s (%d)", e.Message, e.Code)
}

// Message is one CDP JSON object (command, result, or event).
type Message struct {
	ID        int64           `json:"id,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *Error          `json:"error,omitempty"`
}

type listener struct {
	id        int64
	sessionID string
	method    string
	fn        func(json.RawMessage)
}

// Conn is a single CDP websocket with one reader goroutine.
// Flattened target sessions share this connection and are distinguished by sessionId.
type Conn struct {
	ws      *websocket.Conn
	writeMu sync.Mutex
	nextID  atomic.Int64

	mu         sync.Mutex
	pending    map[int64]chan Message
	listeners  []listener
	listenerID atomic.Int64
	closed     chan struct{}
	closeOnce  sync.Once
	err        error
}

// New wraps an existing websocket. The read loop starts immediately.
func New(ws *websocket.Conn) *Conn {
	c := &Conn{
		ws:      ws,
		pending: make(map[int64]chan Message),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c
}

// Dial connects to a browser-level DevTools websocket.
func Dial(wsURL, origin string) (*Conn, error) {
	hdr := http.Header{}
	if origin != "" {
		hdr.Set("Origin", origin)
	}
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		return nil, fmt.Errorf("cdp dial: %w", err)
	}
	return New(ws), nil
}

func debugEnabled() bool {
	return os.Getenv("ECHO_CDP_DEBUG") != ""
}

func (c *Conn) readLoop() {
	defer c.failAll(ErrClosed)
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			c.failAll(err)
			return
		}
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			if debugEnabled() {
				log.Printf("cdp: unmarshal: %v (%s)", err, data)
			}
			continue
		}
		if debugEnabled() && msg.Method != "" {
			log.Printf("cdp event %s session=%s", msg.Method, msg.SessionID)
		}
		if msg.ID != 0 {
			c.mu.Lock()
			ch := c.pending[msg.ID]
			delete(c.pending, msg.ID)
			c.mu.Unlock()
			if ch != nil {
				select {
				case ch <- msg:
				case <-c.closed:
				}
			}
			continue
		}
		c.dispatch(msg)
	}
}

func (c *Conn) dispatch(msg Message) {
	c.mu.Lock()
	ls := append([]listener(nil), c.listeners...)
	c.mu.Unlock()
	for _, l := range ls {
		if l.method != msg.Method {
			continue
		}
		if l.sessionID != msg.SessionID {
			continue
		}
		fn := l.fn
		params := msg.Params
		go fn(params)
	}
}

func (c *Conn) failAll(err error) {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.err = err
		for id, ch := range c.pending {
			delete(c.pending, id)
			close(ch)
		}
		c.mu.Unlock()
		close(c.closed)
		_ = c.ws.Close()
	})
}

// Close shuts the websocket and unblocks pending Calls.
func (c *Conn) Close() error {
	c.failAll(ErrClosed)
	return nil
}

// Closed is signaled when the connection is dead.
func (c *Conn) Closed() <-chan struct{} {
	return c.closed
}

// Call sends a CDP command and waits for its matching id.
// sessionID is empty for the browser session.
func (c *Conn) Call(ctx context.Context, sessionID, method string, params any) (json.RawMessage, error) {
	select {
	case <-c.closed:
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		if err == nil {
			err = ErrClosed
		}
		return nil, err
	default:
	}

	id := c.nextID.Add(1)
	payload := map[string]any{
		"id":     id,
		"method": method,
	}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	if params != nil {
		payload["params"] = params
	}

	ch := make(chan Message, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	if debugEnabled() {
		log.Printf("cdp send id=%d session=%s %s", id, sessionID, method)
	}

	c.writeMu.Lock()
	err := c.ws.WriteJSON(payload)
	c.writeMu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp write %s: %w", method, err)
	}

	select {
	case msg, ok := <-ch:
		if !ok {
			c.mu.Lock()
			err := c.err
			c.mu.Unlock()
			if err == nil {
				err = ErrClosed
			}
			return nil, err
		}
		if msg.Error != nil {
			return nil, msg.Error
		}
		if msg.Result == nil {
			return json.RawMessage(`{}`), nil
		}
		return msg.Result, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp %s: %w", method, ctx.Err())
	}
}

// CallInto is Call plus JSON unmarshal into dest.
func (c *Conn) CallInto(ctx context.Context, sessionID, method string, params, dest any) error {
	raw, err := c.Call(ctx, sessionID, method, params)
	if err != nil {
		return err
	}
	if dest == nil || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("cdp decode %s: %w", method, err)
	}
	return nil
}

// On registers an event handler. An empty sessionID matches browser-session
// events only; pass the target session id for page events.
// The returned function unsubscribes.
func (c *Conn) On(sessionID, method string, fn func(json.RawMessage)) func() {
	id := c.listenerID.Add(1)
	c.mu.Lock()
	c.listeners = append(c.listeners, listener{id: id, sessionID: sessionID, method: method, fn: fn})
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		out := c.listeners[:0]
		for _, l := range c.listeners {
			if l.id != id {
				out = append(out, l)
			}
		}
		c.listeners = out
	}
}
