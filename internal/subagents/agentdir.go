package subagents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ArcheMind/agentx/internal/presets"
)

// AgentMeta is the durable identity of one ExternalSubagent, written by the thin shell at Start
// and updated by its daemon (native session, socket). It is the cold-resume source of truth.
type AgentMeta struct {
	ID              string         `json:"id"`
	Name            string         `json:"name,omitempty"`
	Preset          presets.Preset `json:"preset"`
	Mode            string         `json:"mode"`
	Cwd             string         `json:"cwd"`
	Session         string         `json:"session,omitempty"`
	NativeID        string         `json:"native_id,omitempty"`
	Worktree        *Worktree      `json:"worktree,omitempty"`
	ConnectionID    string         `json:"connection_id"`
	OwnerPID        int            `json:"owner_pid"`
	DelegationDepth int            `json:"delegation_depth"`
	AxBinary        string         `json:"ax_binary,omitempty"`
	Socket          string         `json:"socket"`
	Created         string         `json:"created"`
}

// AgentDir is the daemon state directory of one ExternalSubagent. Note that
// .local/state/sessionman/subagents/<id> already holds Pi session files; the two are never mixed.
func AgentDir(home, id string) string {
	return filepath.Join(home, ".local", "state", "sessionman", "agents", id)
}

func metaPath(dir string) string { return filepath.Join(dir, "meta.json") }

func ReadMeta(dir string) (AgentMeta, error) {
	var meta AgentMeta
	data, err := os.ReadFile(metaPath(dir))
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, fmt.Errorf("invalid agent meta %s: %w", metaPath(dir), err)
	}
	return meta, nil
}

func WriteMeta(dir string, meta AgentMeta) error {
	if meta.Created == "" {
		meta.Created = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return writeFileAtomic(metaPath(dir), data)
}

// socketPath picks the control socket location; macOS rejects sockaddr paths over ~104 bytes,
// so long homes (smoke mkdtemp) fall back to a short path under the temp dir.
func socketPath(dir, id string) string {
	path := filepath.Join(dir, "control.sock")
	if len(path) <= 100 {
		return path
	}
	short := id
	if len(short) > 11 {
		short = short[len(short)-8:]
	}
	return filepath.Join(os.TempDir(), "ax-"+short+".sock")
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
