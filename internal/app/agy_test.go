package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/sessions"
)

func TestAgyConvertSQLite(t *testing.T) {
	var binary bytes.Buffer
	application := App{Output: OutputJSON, Stdin: strings.NewReader(`{"id":"source","provider":"claude","workspace":"/tmp","messages":[{"id":"u","role":"user","content":[{"type":"text","text":"remember this"}]}]}`), Stdout: &binary}
	if err := application.convertTo("agy"); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(binary.Bytes(), []byte("SQLite format 3\x00")) {
		t.Fatal("convert output must be native SQLite")
	}
	var unified bytes.Buffer
	application.Stdin, application.Stdout = bytes.NewReader(binary.Bytes()), &unified
	if err := application.convertFrom("agy"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unified.String(), "remember this") {
		t.Fatal(unified.String())
	}
}

func TestAgyContinueStream(t *testing.T) {
	for _, test := range []struct {
		output    string
		failed    bool
		completed bool
	}{
		{`{"event":"init","conversation_id":"native"}` + "\n" + `{"event":"result","result":{"conversation_id":"native","status":"SUCCESS"}}`, false, true},
		{`{"event":"init","conversation_id":"wrong"}`, true, false},
		{`{"event":"result","result":{"conversation_id":"wrong","status":"SUCCESS"}}`, true, false},
		{`{"event":"result","result":{"conversation_id":"native","status":"ERROR","response":"rejected"}}`, false, false},
	} {
		run, err := parseContinueStream("agy", "future-model", "native", test.output)
		if (err != nil) != test.failed || run.completed != test.completed {
			t.Fatalf("run=%+v err=%v", run, err)
		}
	}
	if _, err := parseContinueStream("unknown", "", "", ""); err == nil {
		t.Fatal("unknown agent accepted")
	}
}

func TestAgyContinueNativePath(t *testing.T) {
	application := App{Sessions: sessions.Service{HomeDir: "/home/native"}}
	path, err := application.continueSessionFile("agy", "/workspace", "id", "")
	if err != nil || path != "/home/native/.gemini/antigravity-cli/conversations/id.db" {
		t.Fatalf("%s %v", path, err)
	}
}
