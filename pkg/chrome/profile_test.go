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
	dir, err := ResolveUserDataDir(`..\..\evil`, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	root, err := filepath.Abs(DefaultProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	got, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, root) {
		t.Fatalf("%s escaped %s", got, root)
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
