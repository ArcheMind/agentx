package app

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/sessions"
)

var terminalControl = regexp.MustCompile(`^\x1b\[(\d*)([A-Za-z])`)

// renderScreen applies the cursor controls the selector emits, so redraw drift shows up as leftover lines.
func renderScreen(output string) []string {
	var lines [][]rune
	row, col := 0, 0
	ensure := func() {
		for len(lines) <= row {
			lines = append(lines, nil)
		}
	}
	for index := 0; index < len(output); {
		if match := terminalControl.FindStringSubmatch(output[index:]); match != nil {
			count, _ := strconv.Atoi(match[1])
			switch match[2] {
			case "A":
				row = max(0, row-max(1, count))
			case "J":
				ensure()
				lines[row] = lines[row][:min(col, len(lines[row]))]
				lines = lines[:row+1]
			case "K":
				ensure()
				lines[row] = nil
			}
			index += len(match[0])
			continue
		}
		r, size := []rune(output[index:])[0], len(string([]rune(output[index:])[0]))
		index += size
		switch r {
		case '\r':
			col = 0
		case '\n':
			row++
		default:
			ensure()
			for len(lines[row]) < col {
				lines[row] = append(lines[row], ' ')
			}
			if col < len(lines[row]) {
				lines[row][col] = r
			} else {
				lines[row] = append(lines[row], r)
			}
			col++
		}
	}
	result := make([]string, len(lines))
	for index, line := range lines {
		result[index] = string(line)
	}
	return result
}

func TestHomeRedrawsInPlace(t *testing.T) {
	application, stdout, _ := presetApp(t, "jjjkkjq")
	mustCreate(t, application.Hooks.Home, deepReview)
	mustCreate(t, application.Hooks.Home, presets.Preset{Name: "daily", Description: "Everyday development", Agent: "claude", Model: "opus"})
	groups := []sessionGroup{{Heading: "Current workspace", Items: []sessions.Summary{{ID: "s1", Provider: "codex", Title: "Fix login state"}}}, {Heading: "Global"}}
	application.chooseHome(bufio.NewReader(application.Stdin), application.Hooks.Home, groups, 0)
	screen := strings.Join(renderScreen(stdout.String()), "\n")
	for _, heading := range []string{"New session · Presets", "Recent sessions · Current workspace", homeHelp} {
		if count := strings.Count(screen, heading); count != 1 {
			t.Fatalf("%q appears %d times on screen:\n%s", heading, count, screen)
		}
	}
}
