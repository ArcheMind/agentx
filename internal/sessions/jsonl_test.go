package sessions

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestJSONLReaders(t *testing.T) {
	large := strings.Repeat("x", 8*1024*1024+1)
	for _, test := range []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty"},
		{name: "LF", input: "{\"text\":\"first\"}\n{\"text\":\"last\"}\n", want: []string{"first", "last"}},
		{name: "CRLF", input: "{\"text\":\"first\"}\r\n{\"text\":\"last\"}\r\n", want: []string{"first", "last"}},
		{name: "no final newline", input: `{"text":"last"}`, want: []string{"last"}},
		{name: "skip invalid lines", input: "\nnot JSON\n[]\n{\"text\":\"valid\"}\n{\"text\":", want: []string{"valid"}},
		{name: "large record", input: "{\"text\":\"" + large + "\"}\n{\"text\":\"last\"}\n", want: []string{large, "last"}},
		{name: "large final record", input: `{"text":"` + large + `"}`, want: []string{large}},
		{name: "large invalid line", input: large + "\n{\"text\":\"valid\"}\n", want: []string{"valid"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.jsonl")
			writeFixture(t, path, test.input)
			for _, reader := range []string{"readJSONL", "scanJSONL"} {
				t.Run(reader, func(t *testing.T) {
					var got []string
					if reader == "readJSONL" {
						records, err := readJSONL(path)
						if err != nil {
							t.Fatal(err)
						}
						for _, record := range records {
							got = append(got, stringValue(record["text"]))
						}
					} else {
						err := scanJSONL(path, func(record map[string]any) bool {
							got = append(got, stringValue(record["text"]))
							return false
						})
						if err != nil {
							t.Fatal(err)
						}
					}
					if !reflect.DeepEqual(got, test.want) {
						t.Fatalf("decoded records differ: got %d records, want %d", len(got), len(test.want))
					}
				})
			}
		})
	}
}

func TestScanJSONLStopsWhenVisitorReturnsTrue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeFixture(t, path, "{\"text\":\"first\"}\n{\"text\":\"second\"}\n")
	visits := 0
	err := scanJSONL(path, func(record map[string]any) bool {
		visits++
		if record["text"] != "first" {
			t.Errorf("visited unexpected record: %v", record)
		}
		return true
	})
	if err != nil || visits != 1 {
		t.Fatalf("visits = %d, err = %v", visits, err)
	}
}

func TestJSONLReadersReportFileErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.jsonl")
	if _, err := readJSONL(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("readJSONL missing file: %v", err)
	}
	if err := scanJSONL(missing, func(map[string]any) bool { return false }); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("scanJSONL missing file: %v", err)
	}
	// Opening a directory may succeed, but reading it must still report an error.
	if _, err := readJSONL(t.TempDir()); err == nil {
		t.Fatal("readJSONL swallowed a directory read error")
	}
	if err := scanJSONL(t.TempDir(), func(map[string]any) bool { return false }); err == nil {
		t.Fatal("scanJSONL swallowed a directory read error")
	}
}

func TestSessionDiscoveryWithLargeJSONLRecords(t *testing.T) {
	home := t.TempDir()
	workspace := "/work/project"
	large := strings.Repeat("x", 8*1024*1024+1)
	path := filepath.Join(home, ".codex", "sessions", "2026", "09", "01", "large.jsonl")
	writeFixture(t, path, strings.Join([]string{
		`{"type":"session_meta","payload":{"id":"large-session","cwd":"/work/project","timestamp":"2026-09-01T10:00:00Z"}}`,
		// A large record before the title must not break recent-session discovery.
		`{"type":"compacted","payload":{"message":"` + large + `"}}`,
		`{"type":"response_item","timestamp":"2026-09-01T10:01:00Z","payload":{"type":"message","role":"user","content":"inspect large output"}}`,
		// The final record has no newline and its full tool output must survive.
		`{"type":"response_item","timestamp":"2026-09-01T10:02:00Z","payload":{"type":"function_call_output","call_id":"call-1","output":"` + large + `"}}`,
	}, "\n"))
	service := Service{HomeDir: home}
	t.Run("list", func(t *testing.T) {
		items, err := service.List(ListOptions{Workspace: workspace})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != "large-session" || items[0].MessageCount != 2 {
			t.Fatalf("sessions = %+v", items)
		}
	})
	t.Run("recent", func(t *testing.T) {
		groups, err := service.RecentGroups(workspace, 10, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(groups.Current) != 1 || groups.Current[0].ID != "large-session" || groups.Current[0].Title != "inspect large output" {
			t.Fatalf("recent sessions = %+v", groups)
		}
	})
	for _, reader := range []string{"info", "native file"} {
		t.Run(reader, func(t *testing.T) {
			var detail Session
			var err error
			if reader == "info" {
				detail, err = service.Info("large-session", "codex")
			} else {
				detail, err = ParseNativeSessionFile("codex", path)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(detail.Messages) != 2 || detail.Messages[1].Role != "tool" || detail.Messages[1].Result != large {
				t.Fatal("large tool output was dropped or truncated")
			}
		})
	}
}
