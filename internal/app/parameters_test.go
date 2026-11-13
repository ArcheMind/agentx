package app

import (
	"bufio"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/presets"
)

func TestCLIAndPresetUseTheSameInvocationParameters(t *testing.T) {
	app, _, runner := presetApp(t, "", "claude")
	disabled := false
	preset := presets.Preset{Name: "shared", Agent: "claude", Model: "opus", Effort: "high", Description: "Same parameters through every entry", UserConfig: &disabled, Flags: []string{"--verbose"}}
	mustCreate(t, app.Hooks.Home, preset)
	if started, err := app.startPreset(context.Background(), bufio.NewReader(strings.NewReader("")), app.Hooks.Home, preset); err != nil || !started {
		t.Fatalf("start = %t, %v", started, err)
	}
	if err := app.Run(context.Background(), []string{"claude", "--model", "opus", "--effort", "high", "--mode", "yolo", "--user-config", "false", "--", "--verbose"}); err != nil {
		t.Fatal(err)
	}
	if len(runner.plans) != 2 {
		t.Fatalf("plans = %#v", runner.plans)
	}
	if !reflect.DeepEqual(runner.plans[0], runner.plans[1]) {
		t.Fatalf("preset = %#v\nCLI = %#v", runner.plans[0], runner.plans[1])
	}
}

func TestInvalidUnifiedParametersDoNotLaunchAnAgent(t *testing.T) {
	app, _, runner := presetApp(t, "", "codex")
	for _, args := range [][]string{
		{"codex", "--mode", "unknown"},
		{"codex", "--user-config", "false"},
		{"codex", "--user-config", "sometimes"},
		{"codex", "--model", "gpt-test", "--", "--model", "other"},
		{"codex", "--mode", "yolo", "--", "--sandbox", "read-only"},
	} {
		if err := app.Run(context.Background(), args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if len(runner.plans) != 0 {
		t.Fatalf("invalid parameters launched: %#v", runner.plans)
	}
}
