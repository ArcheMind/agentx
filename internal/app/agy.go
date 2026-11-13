package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/sessions"
)

func (a App) convertToAgy(detail sessions.Session) error {
	root, err := os.MkdirTemp("", "sessionman-agy-convert-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	path := filepath.Join(root, "conversation.db")
	if err := sessions.WriteAgyDatabaseFile(detail, path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = a.Stdout.Write(data)
	return err
}

func (a App) convertFromAgy(data []byte) error {
	root, err := os.MkdirTemp("", "sessionman-agy-convert-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	path := filepath.Join(root, "conversation.db")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	detail, err := sessions.ParseAgySessionFile(path)
	if err != nil {
		return fmt.Errorf("parse agy SQLite from stdin: %w", err)
	}
	return writeStructured(a.Stdout, detail, a.structuredDefault())
}

func prepareAgyNativePlan(agent string, plan *runtime.CommandPlan) {
	if agent != "agy" {
		return
	}
	if plan.Env == nil {
		plan.Env = map[string]string{}
	}
	plan.Env["SM_MCP_CWD"] = plan.Cwd
}
