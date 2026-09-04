package chrome

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestResolveProxyNoAuth(t *testing.T) {
	s, err := ResolveProxy("http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	if s.ChromeArg != "http://127.0.0.1:8080" {
		t.Fatalf("%s", s.ChromeArg)
	}
	if s.Closer != nil {
		t.Fatal("unexpected forwarder")
	}
	if s.Bypass != "<-loopback>" {
		t.Fatalf("loopback proxy must subtract implicit bypass, got %q", s.Bypass)
	}
}

func TestResolveProxyRemoteNoLoopback(t *testing.T) {
	s, err := ResolveProxy("http://proxy.example.com:8080")
	if err != nil {
		t.Fatal(err)
	}
	if s.ChromeArg != "http://proxy.example.com:8080" {
		t.Fatalf("%s", s.ChromeArg)
	}
	if s.Bypass != "" {
		t.Fatalf("remote proxy should keep Chrome's loopback bypass, got %q", s.Bypass)
	}
}

func TestResolveProxySocks5NoAuth(t *testing.T) {
	s, err := ResolveProxy("socks5://10.0.0.5:1080")
	if err != nil {
		t.Fatal(err)
	}
	if s.ChromeArg != "socks5://10.0.0.5:1080" {
		t.Fatalf("%s", s.ChromeArg)
	}
	if s.Closer != nil {
		t.Fatal("unexpected forwarder")
	}
}

func TestResolveProxyBareHost(t *testing.T) {
	s, err := ResolveProxy("10.1.2.3:8888")
	if err != nil {
		t.Fatal(err)
	}
	if s.ChromeArg != "http://10.1.2.3:8888" {
		t.Fatalf("%s", s.ChromeArg)
	}
}

func TestResolveProxyAuthStartsForwarder(t *testing.T) {
	s, err := ResolveProxy("http://user:pass@127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	if s.Closer == nil {
		t.Fatal("expected forwarder")
	}
	t.Cleanup(func() { _ = s.Closer.Close() })
	if s.ChromeArg == "" || s.ChromeArg == "http://127.0.0.1:8080" {
		t.Fatalf("expected local forwarder, got %s", s.ChromeArg)
	}
	if s.Bypass != "<-loopback>" {
		t.Fatalf("bypass=%q", s.Bypass)
	}
}

func TestResolveProxySocks5AuthStartsForwarder(t *testing.T) {
	s, err := ResolveProxy("socks5://user:pass@10.0.0.5:1080")
	if err != nil {
		t.Fatal(err)
	}
	if s.Closer == nil {
		t.Fatal("expected forwarder")
	}
	t.Cleanup(func() { _ = s.Closer.Close() })
	if !strings.HasPrefix(s.ChromeArg, "http://127.0.0.1:") {
		t.Fatalf("Chrome should see a local HTTP forwarder, got %s", s.ChromeArg)
	}
	if s.Bypass != "<-loopback>" {
		t.Fatalf("bypass=%q", s.Bypass)
	}
}

func TestResolveProxyEmpty(t *testing.T) {
	s, err := ResolveProxy("")
	if err != nil || s.ChromeArg != "" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestResolveProxyUnsupportedScheme(t *testing.T) {
	if _, err := ResolveProxy("ftp://host:21"); err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthForwarderCONNECT(t *testing.T) {
	var mu sync.Mutex
	var gotAuth string
	var gotTarget string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "not connect", http.StatusBadRequest)
			return
		}
		mu.Lock()
		gotAuth = r.Header.Get("Proxy-Authorization")
		gotTarget = r.Host
		mu.Unlock()
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
		if r.Header.Get("Proxy-Authorization") != want {
			http.Error(w, "auth", http.StatusProxyAuthRequired)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("hijack")
		}
		client, _, err := hj.Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		_, _ = io.WriteString(client, "pong")
	}))
	t.Cleanup(upstream.Close)

	u := strings.TrimPrefix(upstream.URL, "http://")
	spec, err := ResolveProxy("http://user:pass@" + u)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = spec.Closer.Close() })

	c, err := net.Dial("tcp", strings.TrimPrefix(spec.ChromeArg, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprintf(c, "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n")
	br := bufio.NewReader(c)
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "200") {
		t.Fatalf("status %q", line)
	}
	for {
		l, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if l == "\r\n" || l == "\n" {
			break
		}
	}
	body := make([]byte, 4)
	if _, err := io.ReadFull(br, body); err != nil {
		t.Fatal(err)
	}
	if string(body) != "pong" {
		t.Fatalf("body %q", body)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("auth %q", gotAuth)
	}
	if gotTarget != "example.com:443" {
		t.Fatalf("target %q", gotTarget)
	}
}

func TestHTTPConnectLeftover(t *testing.T) {
	srv, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	go func() {
		c, err := srv.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			if line == "\r\n" {
				break
			}
		}
		_, _ = io.WriteString(c, "HTTP/1.1 200 Connection Established\r\n\r\nHELLO")
	}()

	c, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	leftover, err := httpConnect(c, "example.com:443", "Basic dXNlcjpwYXNz")
	if err != nil {
		t.Fatal(err)
	}
	if string(leftover) != "HELLO" {
		t.Fatalf("leftover %q", leftover)
	}
}

func TestConnectOK(t *testing.T) {
	if !connectOK("HTTP/1.1 200 Connection Established") {
		t.Fatal("200")
	}
	if connectOK("HTTP/1.1 407 Proxy Authentication Required") {
		t.Fatal("407")
	}
}

func TestSocks5ConnectAuth(t *testing.T) {
	a, b := net.Pipe()
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	errCh := make(chan error, 1)
	go func() {
		errCh <- socks5Connect(a, "example.com:443", "user", "pass")
	}()

	buf := make([]byte, 4)
	if _, err := io.ReadFull(b, buf); err != nil {
		t.Fatal(err)
	}
	if buf[0] != 5 || buf[1] != 2 {
		t.Fatalf("greeting %v", buf)
	}
	_, _ = b.Write([]byte{0x05, 0x02})
	ver := make([]byte, 1)
	if _, err := io.ReadFull(b, ver); err != nil {
		t.Fatal(err)
	}
	if ver[0] != 1 {
		t.Fatalf("auth ver %v", ver)
	}
	ulen := make([]byte, 1)
	if _, err := io.ReadFull(b, ulen); err != nil {
		t.Fatal(err)
	}
	u := make([]byte, int(ulen[0]))
	if _, err := io.ReadFull(b, u); err != nil {
		t.Fatal(err)
	}
	plen := make([]byte, 1)
	if _, err := io.ReadFull(b, plen); err != nil {
		t.Fatal(err)
	}
	p := make([]byte, int(plen[0]))
	if _, err := io.ReadFull(b, p); err != nil {
		t.Fatal(err)
	}
	if string(u) != "user" || string(p) != "pass" {
		t.Fatalf("%s %s", u, p)
	}
	_, _ = b.Write([]byte{0x01, 0x00})

	hdr := make([]byte, 4)
	if _, err := io.ReadFull(b, hdr); err != nil {
		t.Fatal(err)
	}
	if hdr[0] != 5 || hdr[1] != 1 || hdr[3] != 3 {
		t.Fatalf("req %v", hdr)
	}
	nlen := make([]byte, 1)
	if _, err := io.ReadFull(b, nlen); err != nil {
		t.Fatal(err)
	}
	name := make([]byte, int(nlen[0])+2)
	if _, err := io.ReadFull(b, name); err != nil {
		t.Fatal(err)
	}
	if string(name[:nlen[0]]) != "example.com" {
		t.Fatalf("host %q", name)
	}
	_, _ = b.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}
