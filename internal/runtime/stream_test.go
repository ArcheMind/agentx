package runtime

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func TestStreamStderrPrecedesCompletion(t *testing.T) {
	stream, err := (ExecRunner{Log: io.Discard}).StartStream(CommandPlan{
		Executable: os.Args[0],
		Args:       []string{"-test.run=^TestStreamStderrHelperProcess$"},
		Env:        map[string]string{"SM_TEST_STREAM_HELPER": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stream.Kill(); _ = stream.Wait() })
	path := stream.stderr.Name()
	var expected strings.Builder
	for i := 0; i < 200; i++ {
		if line, ok := <-stream.Lines(); !ok || line != "completed" {
			t.Fatalf("completion %d: %q, open=%t", i, line, ok)
		}
		fmt.Fprintf(&expected, "evidence:%03d\n", i)
		if got := stream.Stderr(); got != expected.String() {
			t.Fatalf("completion %d observed before stderr: got %q", i, got)
		}
		if err := stream.WriteLine("next"); err != nil {
			t.Fatal(err)
		}
	}
	for range stream.Lines() {
	}
	if err := stream.Wait(); err != nil {
		t.Fatal(err)
	}
	if got := stream.Stderr(); got != expected.String() {
		t.Fatal("stderr was lost after process exit")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("capture was not cleaned up: %v", err)
	}
}

func TestStreamStderrHelperProcess(t *testing.T) {
	if os.Getenv("SM_TEST_STREAM_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for i := 0; i < 200; i++ {
		fmt.Fprintf(os.Stderr, "evidence:%03d\n", i)
		fmt.Fprintln(os.Stdout, "completed")
		if !scanner.Scan() {
			os.Exit(2)
		}
	}
	os.Exit(0)
}
