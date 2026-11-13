package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestAgyOwnedPluginConcurrentLaunches(t *testing.T) {
	home := t.TempDir()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := PrepareAgyPlugin(home, "/bin/sm", true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	root, err := PrepareAgyPlugin(home, "/bin/new-sm", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"plugin.json", "mcp_config.json", "hooks.json", ".sessionman-owned"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var hooks map[string]map[string][]struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(data, &hooks); err != nil {
		t.Fatal(err)
	}
	if len(hooks["sessionman"]) != 2 {
		t.Fatalf("hooks = %+v", hooks)
	}
	for _, event := range []string{"SessionStart", "Stop"} {
		handlers := hooks["sessionman"][event]
		if len(handlers) != 1 || handlers[0].Command != agyShellExecutable("/bin/new-sm", runtime.GOOS)+" hook __agy_dispatch --event "+event {
			t.Fatalf("%s hooks = %+v", event, handlers)
		}
	}
}

func TestAgyPluginForeignCollision(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".gemini", "config", "plugins", "sessionman")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareAgyPlugin(home, "/bin/sm", true); err == nil {
		t.Fatal("foreign plugin overwritten")
	}
}

func TestAgyHookExecutableQuoting(t *testing.T) {
	cases := []struct{ value, platform, want string }{
		{`C:\Program Files\SessionMan\sm.exe`, "windows", `"C:\Program Files\SessionMan\sm.exe"`},
		{"/Applications/Session Man's/sm", "darwin", "'/Applications/Session Man'\"'\"'s/sm'"},
	}
	for _, test := range cases {
		if got := agyShellExecutable(test.value, test.platform); got != test.want {
			t.Fatalf("got %q want %q", got, test.want)
		}
	}
}
