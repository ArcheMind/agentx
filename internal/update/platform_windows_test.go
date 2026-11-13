package update

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if handled, err := RunHelper(os.Args[1:], os.Stdout, os.Stderr); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestWindowsUpdateParent(t *testing.T) {
	if os.Getenv("SM_UPDATE_PLATFORM_PARENT") != "1" {
		t.Skip("subprocess helper")
	}
	handedOff, err := replaceExecutable(os.Getenv("SM_UPDATE_PLATFORM_STAGED"), os.Getenv("SM_UPDATE_PLATFORM_TARGET"), "9.9.9", os.Stdout, os.Stderr, true)
	if err != nil || !handedOff {
		fmt.Fprintf(os.Stderr, "handoff = %v, %v\n", handedOff, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestWindowsHelperUpdatesRunningExecutableAndRollsBack(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	for _, success := range []bool{true, false} {
		t.Run(fmt.Sprintf("success=%v", success), func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "sm.exe")
			staged := filepath.Join(dir, ".sm-update-new.exe")
			if err := copyHelper(executable, target); err != nil {
				t.Fatal(err)
			}
			if success {
				if err := os.WriteFile(staged, []byte("replacement"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			logPath := filepath.Join(dir, "helper.log")
			logFile, err := os.Create(logPath)
			if err != nil {
				t.Fatal(err)
			}
			defer logFile.Close()
			command := exec.Command(target, "-test.run=^TestWindowsUpdateParent$")
			command.Env = append(os.Environ(), "SM_UPDATE_PLATFORM_PARENT=1", "SM_UPDATE_PLATFORM_TARGET="+target, "SM_UPDATE_PLATFORM_STAGED="+staged)
			command.Stdout = logFile
			command.Stderr = logFile
			if err := command.Run(); err != nil {
				log, _ := os.ReadFile(logPath)
				t.Fatalf("parent failed: %v\n%s", err, log)
			}
			marker := "Updated SessionMan to 9.9.9."
			if !success {
				marker = "original restored"
			}
			deadline := time.Now().Add(10 * time.Second)
			for {
				log, err := os.ReadFile(logPath)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(log), marker) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("helper did not finish: %s", log)
				}
				time.Sleep(20 * time.Millisecond)
			}
			installed, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			want := original
			if success {
				want = []byte("replacement")
			}
			if !bytes.Equal(installed, want) {
				t.Fatalf("installed executable has unexpected contents (%d bytes)", len(installed))
			}
			if _, err := os.Stat(staged); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("staged file remains: %v", err)
			}
		})
	}
}

func TestFinishReplacementRestoresOriginalOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sm.exe")
	staged := filepath.Join(dir, ".sm-update-new.exe")
	if err := os.WriteFile(target, []byte("original"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := finishReplacement(staged, target); err == nil {
		t.Fatal("missing staged file did not fail")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "original" {
		t.Fatalf("original was not restored: %q, %v", data, err)
	}
	if err := os.WriteFile(staged, []byte("replacement"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := finishReplacement(staged, target); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(target)
	if err != nil || string(data) != "replacement" {
		t.Fatalf("installed = %q, %v", data, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("update left backup or staged files: %v, %v", entries, err)
	}
}

func TestRunHelperRejectsOrdinaryAndMalformedArguments(t *testing.T) {
	if handled, err := RunHelper([]string{"version"}, io.Discard, io.Discard); handled || err != nil {
		t.Fatalf("ordinary command handled = %v, %v", handled, err)
	}
	for _, args := range [][]string{
		{helperFlag},
		{helperFlag, "invalid", "staged", "target", "1.2.3", "temp"},
		{helperFlag, "0", "staged", "target", "1.2.3", "temp"},
	} {
		if handled, err := RunHelper(args, io.Discard, io.Discard); !handled || err == nil {
			t.Fatalf("malformed command %v handled = %v, %v", args, handled, err)
		}
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
