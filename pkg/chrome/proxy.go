package chrome

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProxySpec is a Chrome --proxy-server value plus optional local auth helper.
type ProxySpec struct {
	// ChromeArg is passed to --proxy-server (no credentials).
	ChromeArg string
	// Bypass is passed to --proxy-bypass-list. "<-loopback>" is required when
	// ChromeArg points at 127.0.0.1 (Chrome otherwise skips the proxy).
	Bypass string
	// Closer shuts down a local auth forwarder, if any.
	Closer io.Closer
}

// ResolveProxy parses a proxy URL. user:pass@host is rewritten through a local
// CONNECT forwarder so Chrome's TLS to the origin is unchanged.
func ResolveProxy(raw string) (ProxySpec, error) {
	if strings.TrimSpace(raw) == "" {
		return ProxySpec{}, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ProxySpec{}, fmt.Errorf("chrome: proxy: %w", err)
	}
	if u.Host == "" {
		return ProxySpec{}, fmt.Errorf("chrome: proxy: missing host")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		scheme = "http"
	}
	switch scheme {
	case "http", "https", "socks", "socks5":
	default:
		return ProxySpec{}, fmt.Errorf("chrome: proxy: unsupported scheme %s", scheme)
	}
	user := ""
	pass := ""
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}
	if user == "" {
		return ProxySpec{
			ChromeArg: scheme + "://" + u.Host,
			Bypass:    loopbackBypass(u.Host),
		}, nil
	}
	fwd, err := newAuthForwarder(scheme, u.Host, user, pass)
	if err != nil {
		return ProxySpec{}, err
	}
	return ProxySpec{
		ChromeArg: "http://" + fwd.Addr(),
		Bypass:    "<-loopback>",
		Closer:    fwd,
	}, nil
}

func loopbackBypass(hostport string) string {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if strings.EqualFold(host, "localhost") || strings.EqualFold(host, "localhost.") || (ip != nil && ip.IsLoopback()) {
		return "<-loopback>"
	}
	return ""
}

type authForwarder struct {
	ln     net.Listener
	scheme string
	host   string
	user   string
	pass   string
	auth   string
	wg     sync.WaitGroup
}

func newAuthForwarder(scheme, upstreamHost, user, pass string) (*authForwarder, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("chrome: proxy listen: %w", err)
	}
	token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	f := &authForwarder{
		ln:     ln,
		scheme: scheme,
		host:   upstreamHost,
		user:   user,
		pass:   pass,
		auth:   "Basic " + token,
	}
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		_ = http.Serve(ln, f)
	}()
	return f, nil
}

func (f *authForwarder) Addr() string { return f.ln.Addr().String() }

func (f *authForwarder) Close() error {
	err := f.ln.Close()
	f.wg.Wait()
	return err
}

func (f *authForwarder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		f.handleConnect(w, r)
		return
	}
	f.handleHTTP(w, r)
}

func (f *authForwarder) socks() bool {
	return f.scheme == "socks5" || f.scheme == "socks"
}

func (f *authForwarder) handleConnect(w http.ResponseWriter, r *http.Request) {
	up, err := f.openTunnel(r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer up.Close()

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	client, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	flushBuffered(rw, up)
	pipe(client, up)
}

func (f *authForwarder) handleHTTP(w http.ResponseWriter, r *http.Request) {
	var up net.Conn
	var err error
	if f.socks() {
		host := r.URL.Host
		if host == "" {
			host = r.Host
		}
		if _, _, splitErr := net.SplitHostPort(host); splitErr != nil {
			host = net.JoinHostPort(host, "80")
		}
		up, err = f.openTunnel(host)
	} else {
		up, err = f.dialUpstream()
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer up.Close()

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	client, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer client.Close()

	r.Header.Del("Proxy-Connection")
	if !f.socks() && f.auth != "" {
		r.Header.Set("Proxy-Authorization", f.auth)
	}
	if f.socks() {
		if err := r.Write(up); err != nil {
			return
		}
	} else if err := r.WriteProxy(up); err != nil {
		return
	}
	flushBuffered(rw, up)
	pipe(client, up)
}

func flushBuffered(rw *bufio.ReadWriter, up net.Conn) {
	if rw == nil {
		return
	}
	n := rw.Reader.Buffered()
	if n <= 0 {
		return
	}
	buf := make([]byte, n)
	_, _ = rw.Read(buf)
	_, _ = up.Write(buf)
}

func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(a, b)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(b, a)
		done <- struct{}{}
	}()
	<-done
	now := time.Now()
	_ = a.SetDeadline(now)
	_ = b.SetDeadline(now)
	<-done
}

func (f *authForwarder) dialUpstream() (net.Conn, error) {
	addr := withDefaultPort(f.host, f.scheme)
	if f.scheme == "https" {
		serverName, _, err := net.SplitHostPort(addr)
		if err != nil {
			serverName = f.host
		}
		return tls.Dial("tcp", addr, &tls.Config{ServerName: serverName})
	}
	return net.Dial("tcp", addr)
}

func (f *authForwarder) openTunnel(target string) (net.Conn, error) {
	if f.socks() {
		c, err := net.Dial("tcp", withDefaultPort(f.host, "socks5"))
		if err != nil {
			return nil, err
		}
		if err := socks5Connect(c, target, f.user, f.pass); err != nil {
			_ = c.Close()
			return nil, err
		}
		return c, nil
	}
	c, err := f.dialUpstream()
	if err != nil {
		return nil, err
	}
	leftover, err := httpConnect(c, target, f.auth)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	if len(leftover) > 0 {
		return &prefixConn{Conn: c, prefix: leftover}, nil
	}
	return c, nil
}

func withDefaultPort(host, scheme string) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	port := "8080"
	switch scheme {
	case "https":
		port = "443"
	case "socks", "socks5":
		port = "1080"
	}
	return net.JoinHostPort(host, port)
}

func httpConnect(c net.Conn, target, auth string) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
	if auth != "" {
		fmt.Fprintf(&b, "Proxy-Authorization: %s\r\n", auth)
	}
	b.WriteString("Proxy-Connection: Keep-Alive\r\n\r\n")
	if _, err := io.WriteString(c, b.String()); err != nil {
		return nil, err
	}
	status, leftover, err := readHTTPHead(c)
	if err != nil {
		return nil, err
	}
	if !connectOK(status) {
		return nil, fmt.Errorf("upstream CONNECT failed: %s", status)
	}
	return leftover, nil
}

func readHTTPHead(r io.Reader) (statusLine string, leftover []byte, err error) {
	var buf []byte
	tmp := make([]byte, 1024)
	for {
		n, readErr := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if i := bytes.Index(buf, []byte("\r\n\r\n")); i >= 0 {
			head := buf[:i]
			leftover = append([]byte(nil), buf[i+4:]...)
			line, _, _ := strings.Cut(string(head), "\r\n")
			return line, leftover, nil
		}
		if readErr != nil {
			if readErr == io.EOF && len(buf) > 0 {
				return "", nil, fmt.Errorf("truncated CONNECT response")
			}
			return "", nil, readErr
		}
		if len(buf) > 64*1024 {
			return "", nil, fmt.Errorf("CONNECT response too large")
		}
	}
}

func connectOK(statusLine string) bool {
	parts := strings.SplitN(statusLine, " ", 3)
	return len(parts) >= 2 && parts[1] == "200"
}

type prefixConn struct {
	net.Conn
	prefix []byte
}

func (c *prefixConn) Read(p []byte) (int, error) {
	if len(c.prefix) > 0 {
		n := copy(p, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}

func socks5Connect(c net.Conn, target, user, pass string) error {
	if user != "" {
		if _, err := c.Write([]byte{0x05, 0x02, 0x00, 0x02}); err != nil {
			return err
		}
	} else if _, err := c.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(c, reply); err != nil {
		return err
	}
	if reply[0] != 5 {
		return fmt.Errorf("socks: bad version %d", reply[0])
	}
	switch reply[1] {
	case 0x00:
	case 0x02:
		ub, pb := []byte(user), []byte(pass)
		if len(ub) > 255 || len(pb) > 255 {
			return fmt.Errorf("socks: credentials too long")
		}
		req := make([]byte, 0, 3+len(ub)+len(pb))
		req = append(req, 0x01, byte(len(ub)))
		req = append(req, ub...)
		req = append(req, byte(len(pb)))
		req = append(req, pb...)
		if _, err := c.Write(req); err != nil {
			return err
		}
		ar := make([]byte, 2)
		if _, err := io.ReadFull(c, ar); err != nil {
			return err
		}
		if ar[1] != 0 {
			return fmt.Errorf("socks: auth failed")
		}
	default:
		return fmt.Errorf("socks: unsupported method %d", reply[1])
	}

	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("socks: target: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		return fmt.Errorf("socks: bad port %s", portStr)
	}

	req := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			req = append(req, 0x01)
			req = append(req, v4...)
		} else {
			req = append(req, 0x04)
			req = append(req, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return fmt.Errorf("socks: hostname too long")
		}
		req = append(req, 0x03, byte(len(host)))
		req = append(req, host...)
	}
	req = append(req, byte(port>>8), byte(port))
	if _, err := c.Write(req); err != nil {
		return err
	}

	hdr := make([]byte, 4)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return err
	}
	if hdr[0] != 5 {
		return fmt.Errorf("socks: bad reply version")
	}
	if hdr[1] != 0 {
		return fmt.Errorf("socks: connect status %d", hdr[1])
	}
	switch hdr[3] {
	case 1:
		_, err = io.ReadFull(c, make([]byte, 6))
	case 4:
		_, err = io.ReadFull(c, make([]byte, 18))
	case 3:
		l := make([]byte, 1)
		if _, err = io.ReadFull(c, l); err != nil {
			return err
		}
		_, err = io.ReadFull(c, make([]byte, int(l[0])+2))
	default:
		return fmt.Errorf("socks: bad atyp %d", hdr[3])
	}
	return err
}
