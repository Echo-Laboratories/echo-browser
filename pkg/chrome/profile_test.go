package chrome

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRejectsTraversal(t *testing.T) {
	if _, err := ResolveUserDataDir("..", false); err == nil {
		t.Fatal("expected error for ..")
	}
	root, err := filepath.Abs(DefaultProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{`..\..\evil`, `../../evil`} {
		dir, err := ResolveUserDataDir(name, false)
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		got, err := filepath.Abs(dir)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(root, got)
		if err != nil || strings.HasPrefix(rel, "..") {
			t.Fatalf("%q escaped %s -> %s", name, root, got)
		}
	}
}

func TestResolvePersistentCreatesDir(t *testing.T) {
	dir, err := ResolveUserDataDir("unit-test-profile", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if !strings.Contains(filepath.ToSlash(dir), "unit-test-profile") {
		t.Fatalf("dir=%s", dir)
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		t.Fatalf("stat %v %v", st, err)
	}
}

func TestResolveEphemeral(t *testing.T) {
	dir, err := ResolveUserDataDir("ignored", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}
