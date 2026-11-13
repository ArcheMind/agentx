package sessions

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubagentMarkerDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name   string
		first  string
		second string
		child  bool
	}{
		{"marked", "[sm:subagent]\ninspect", "continue", true},
		{"crlf", "[sm:subagent]\r\ninspect", "continue", true},
		{"ordinary", "inspect", "continue", false},
		{"quoted", "Explain [sm:subagent]", "continue", false},
		{"different-marker", "[sm:subagent]extra\ninspect", "continue", false},
		{"later-message", "inspect", "[sm:subagent]\ncontinue", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, workspace := t.TempDir(), t.TempDir()
			writeFixture(t, filepath.Join(home, ".claude", "projects", "project", "child.jsonl"), strings.Join([]string{
				fmt.Sprintf(`{"type":"user","sessionId":"claude-child","cwd":%q,"message":{"content":%q}}`, workspace, tc.first),
				fmt.Sprintf(`{"type":"user","message":{"content":%q}}`, tc.second),
			}, "\n"))
			writeFixture(t, filepath.Join(home, ".codex", "sessions", "2026", "11", "13", "child.jsonl"), strings.Join([]string{
				fmt.Sprintf(`{"type":"session_meta","payload":{"id":"codex-child","cwd":%q,"source":"cli"}}`, workspace),
				`{"type":"response_item","payload":{"type":"message","role":"user","content":"# AGENTS.md instructions\nProject rules"}}`,
				`{"type":"response_item","payload":{"type":"message","role":"user","content":"<environment_context>Workspace</environment_context>"}}`,
				fmt.Sprintf(`{"type":"response_item","payload":{"type":"message","role":"user","content":%q}}`, tc.first),
				fmt.Sprintf(`{"type":"response_item","payload":{"type":"message","role":"user","content":%q}}`, tc.second),
			}, "\n"))
			writeFixture(t, filepath.Join(home, ".pi", "agent", "sessions", "project", "child.jsonl"), strings.Join([]string{
				fmt.Sprintf(`{"type":"session","id":"pi-child","cwd":%q}`, workspace),
				fmt.Sprintf(`{"type":"message","id":"first","message":{"role":"user","content":%q}}`, tc.first),
				fmt.Sprintf(`{"type":"message","id":"second","parentId":"first","message":{"role":"user","content":%q}}`, tc.second),
			}, "\n"))
			root := filepath.Join(home, ".local", "share", "opencode", "project", "project", "storage", "session")
			writeFixture(t, filepath.Join(root, "info", "opencode-child.json"), `{"id":"opencode-child","title":"Existing title"}`)
			for i, text := range []string{tc.first, tc.second} {
				id := fmt.Sprintf("message-%d", i)
				writeFixture(t, filepath.Join(root, "message", "opencode-child", id+".json"),
					fmt.Sprintf(`{"id":%q,"role":"user","path":{"cwd":%q},"time":{"created":%d}}`, id, workspace, i+1))
				writeFixture(t, filepath.Join(root, "part", "opencode-child", id, "part.json"), fmt.Sprintf(`{"type":"text","text":%q}`, text))
			}

			service := Service{HomeDir: home}
			for _, include := range []bool{false, true} {
				want := 4
				if tc.child && !include {
					want = 0
				}
				items, err := service.List(ListOptions{All: true, IncludeSubagents: include})
				if err != nil {
					t.Fatal(err)
				}
				groups, err := service.RecentGroups(workspace, 10, include)
				if err != nil {
					t.Fatal(err)
				}
				global, err := service.RecentGroups(t.TempDir(), 10, include)
				if err != nil {
					t.Fatal(err)
				}
				for _, list := range [][]Summary{items, groups.Current, global.Global} {
					if len(list) != want {
						t.Fatalf("include=%v: got %d sessions, want %d: %+v", include, len(list), want, list)
					}
					for _, item := range list {
						if item.IsSubagent != tc.child {
							t.Fatalf("include=%v: incorrect identity: %+v", include, item)
						}
					}
				}
			}
		})
	}
}
