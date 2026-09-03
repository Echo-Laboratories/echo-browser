package chrome

import "testing"

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
}

func TestResolveProxyEmpty(t *testing.T) {
	s, err := ResolveProxy("")
	if err != nil || s.ChromeArg != "" {
		t.Fatalf("%+v %v", s, err)
	}
}
