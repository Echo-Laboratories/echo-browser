package chrome

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// ProxySpec is a Chrome --proxy-server value plus optional local auth helper.
type ProxySpec struct {
	// ChromeArg is passed to --proxy-server (no credentials).
	ChromeArg string
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
	user := ""
	pass := ""
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		scheme = "http"
	}
	if user == "" {
		return ProxySpec{ChromeArg: scheme + "://" + u.Host}, nil
	}
	if scheme != "http" && scheme != "https" {
		return ProxySpec{}, fmt.Errorf("chrome: authenticated proxy only supported for http(s), not %s", scheme)
	}
	fwd, err := newAuthForwarder(u.Host, user, pass)
	if err != nil {
		return ProxySpec{}, err
	}
	return ProxySpec{
		ChromeArg: "http://" + fwd.Addr(),
		Closer:    fwd,
	}, nil
}

type authForwarder struct {
	ln   net.Listener
	host string
	auth string
	wg   sync.WaitGroup
}

func newAuthForwarder(upstreamHost, user, pass string) (*authForwarder, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("chrome: proxy listen: %w", err)
	}
	token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	f := &authForwarder{ln: ln, host: upstreamHost, auth: "Basic " + token}
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

func (f *authForwarder) handleConnect(w http.ResponseWriter, r *http.Request) {
	up, err := net.Dial("tcp", f.host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer up.Close()

	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\nProxy-Connection: Keep-Alive\r\n\r\n", r.Host, r.Host, f.auth)
	if _, err := io.WriteString(up, req); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	buf := make([]byte, 4096)
	n, err := up.Read(buf)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !strings.Contains(string(buf[:n]), " 200 ") {
		http.Error(w, "upstream CONNECT failed: "+string(buf[:n]), http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	client, _, err := hj.Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
	go func() { _, _ = io.Copy(up, client) }()
	_, _ = io.Copy(client, up)
}

func (f *authForwarder) handleHTTP(w http.ResponseWriter, r *http.Request) {
	r.RequestURI = ""
	r.Header.Set("Proxy-Authorization", f.auth)
	transport := &http.Transport{
		Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: f.host}),
	}
	resp, err := transport.RoundTrip(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
