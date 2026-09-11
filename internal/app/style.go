package app

import (
	"io"
	"os"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

var agentColors = map[string]string{
	"claude":   "\x1b[38;5;208m",
	"codex":    "\x1b[38;5;42m",
	"dsh":      "\x1b[38;5;33m",
	"gemini":   "\x1b[38;5;45m",
	"opencode": "\x1b[38;5;220m",
	"pi":       "\x1b[38;5;201m",
}

func colorEnabled(writer io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func colorize(enabled bool, color, value string) string {
	if !enabled || color == "" {
		return value
	}
	return color + value + ansiReset
}

func agentColor(provider string) string {
	return agentColors[provider]
}
