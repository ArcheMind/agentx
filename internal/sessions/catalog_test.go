package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSessionOperationsShareCatalog(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	day := filepath.Join(home, ".codex", "sessions", "2026", "11", "13")
	writeRollout := func(name, id, timestamp, text string) string {
		path := filepath.Join(day, name+".jsonl")
		writeFixture(t, path, strings.Join([]string{
			fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"cwd":%q,"timestamp":"2026-11-13T08:00:00Z","source":"cli"}}`, id, workspace),
			fmt.Sprintf(`{"type":"response_item","timestamp":%q,"payload":{"type":"message","role":"user","content":%q}}`, timestamp, text),
		}, "\n"))
		return path
	}
	old := writeRollout("rollout-shared", "shared", "2026-11-13T08:01:00Z", "original request")
	writeRollout("rollout-shared_segment-one", "shared", "2026-11-13T08:02:00Z", "first continuation")
	latest := writeRollout("rollout-shared_segment-two", "shared", "2026-11-13T08:03:00Z", "latest continuation")
	other := writeRollout("rollout-other", "other", "2026-11-13T08:00:30Z", "another session")
	// File modification times must not override the session's record timestamps.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(old, future, future); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(home, ".claude", "projects", "project", "shared.jsonl"),
		fmt.Sprintf(`{"type":"user","sessionId":"shared","cwd":%q,"timestamp":"2026-11-13T08:00:00Z","message":{"content":"different provider"}}`, workspace))

	service := Service{HomeDir: home}
	list, err := service.List(ListOptions{Workspace: workspace, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.RecentGroups(workspace, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	global, err := service.RecentGroups(t.TempDir(), 2, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, items := range [][]Summary{list, current.Current, global.Global} {
		if len(items) != 3 {
			t.Fatalf("duplicate rollouts consumed the limit or merged providers: %+v", items)
		}
		for i, item := range items {
			want := list[i]
			if item.Agent != want.Agent || item.ID != want.ID || item.Source != want.Source ||
				item.StartedAt != want.StartedAt || item.UpdatedAt != want.UpdatedAt ||
				item.FirstUserMessage != want.FirstUserMessage || item.LatestUserMessage != want.LatestUserMessage {
				t.Fatalf("session data differs across views: got %+v, want %+v", item, want)
			}
		}
	}
	if list[0].Source != latest || list[1].Source != other {
		t.Fatalf("incorrect latest-record selection: %+v", list)
	}
	detail, err := service.Info("shared", "codex")
	if err != nil || detail.Source != latest || MessageText(detail.Messages[0]) != "latest continuation" {
		t.Fatalf("info = %+v, err = %v", detail, err)
	}
	matches, err := service.Matches("shar", "codex")
	if err != nil || len(matches) != 1 || !reflect.DeepEqual(matches[0], detail) {
		t.Fatalf("matches = %+v, err = %v", matches, err)
	}
	if _, err := service.Info("shared", ""); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("provider ambiguity was lost: %v", err)
	}
	path, err := FindCodexRollout(filepath.Join(home, ".codex"), "shared")
	if err != nil || path != latest {
		t.Fatalf("continue path = %q, want %q, err = %v", path, latest, err)
	}
	if _, err := FindCodexRollout(filepath.Join(home, ".codex"), "missing"); err == nil {
		t.Fatal("missing session resolved to a rollout")
	}
}
