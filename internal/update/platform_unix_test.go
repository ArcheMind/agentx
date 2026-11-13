//go:build !windows

package update

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceExecutable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sm")
	staged := filepath.Join(dir, ".sm-update-new")
	if err := os.WriteFile(target, []byte("original"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("replacement"), 0755); err != nil {
		t.Fatal(err)
	}
	handedOff, err := replaceExecutable(staged, target, "1.2.3", io.Discard, io.Discard, false)
	if err != nil || handedOff {
		t.Fatalf("replace = %v, %v", handedOff, err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "replacement" {
		t.Fatalf("installed = %q, %v", data, err)
	}
	if _, err := os.Stat(staged); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged file remains: %v", err)
	}
	if _, err := replaceExecutable(staged, target, "1.2.3", io.Discard, io.Discard, false); err == nil {
		t.Fatal("missing staged file did not fail")
	}
	data, err = os.ReadFile(target)
	if err != nil || string(data) != "replacement" {
		t.Fatalf("failed update changed installed executable: %q, %v", data, err)
	}
}

func TestCacheLockDoesNotWaitAndReleasesAfterError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.lock")
	want := errors.New("callback failed")
	err := withCacheLock(path, func() error {
		called := false
		if err := withCacheLock(path, func() error { called = true; return nil }); err == nil || called {
			t.Fatal("contended cache lock was acquired")
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("callback error = %v", err)
	}
	if err := withCacheLock(path, func() error { return nil }); err != nil {
		t.Fatalf("cache lock was not released: %v", err)
	}
}
