package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArcheMind/agentx/internal/sessions"
	"golang.org/x/term"
)

const recentSessionsPerProvider = 10

type sessionGroup struct {
	Heading string
	Items   []sessions.Summary
}

func (a App) recentSessionGroups() ([]sessionGroup, error) {
	items, err := a.Sessions.RecentGroups("", recentSessionsPerProvider)
	if err != nil {
		return nil, err
	}
	return []sessionGroup{
		{Heading: "Current workspace", Items: items.Current},
		{Heading: "Global", Items: items.Global},
	}, nil
}

func (a App) chooseSession(reader *bufio.Reader, groups []sessionGroup) (sessions.Summary, error) {
	items := make([]sessions.Summary, 0)
	for _, group := range groups {
		items = append(items, group.Items...)
	}

	selected := 0
	lineCount := 0
	now := time.Now()
	render := func(clear bool) {
		if clear {
			fmt.Fprintf(a.Stdout, "\x1b[%dA\r\x1b[J", lineCount)
		}
		lineCount = 2
		fmt.Fprint(a.Stdout, "Choose a recent session:\r\n")
		fmt.Fprint(a.Stdout, "Use ↑/↓ to move, Enter to select, q to cancel.\r\n")
		itemIndex := 0
		for _, group := range groups {
			fmt.Fprintf(a.Stdout, "\r\n%s\r\n", group.Heading)
			lineCount += 2
			if len(group.Items) == 0 {
				fmt.Fprint(a.Stdout, "  No recent sessions\r\n")
				lineCount++
				continue
			}
			for _, item := range group.Items {
				marker := "  "
				if itemIndex == selected {
					marker = "> "
				}
				title := item.Title
				if title == "" {
					title = item.ID
				}
				updated := displaySessionTime(item.UpdatedAt, now)
				if group.Heading == "Global" {
					fmt.Fprintf(a.Stdout, "%s%-8s  %-18s  %s\r\n", marker, item.Provider, updated, singleLine(title, 52))
					fmt.Fprintf(a.Stdout, "            ↳ %s\r\n", displayWorkspace(item.Workspace, 72))
					lineCount++
				} else {
					fmt.Fprintf(a.Stdout, "%s%-8s  %-18s  %s\r\n", marker, item.Provider, updated, singleLine(title, 52))
				}
				lineCount++
				itemIndex++
			}
		}
	}

	render(false)
	if len(items) == 0 {
		return sessions.Summary{}, errors.New("no recent sessions found in the current workspace or globally")
	}

	restore, err := makeRaw(a.Stdin)
	if err != nil {
		return sessions.Summary{}, err
	}
	defer restore()

	for {
		key, err := readSelectionKey(reader)
		if err != nil {
			return sessions.Summary{}, err
		}
		switch key {
		case selectionUp:
			if selected > 0 {
				selected--
				render(true)
			}
		case selectionDown:
			if selected < len(items)-1 {
				selected++
				render(true)
			}
		case selectionConfirm:
			fmt.Fprint(a.Stdout, "\r\n")
			return items[selected], nil
		case selectionCancel:
			fmt.Fprint(a.Stdout, "\r\n")
			return sessions.Summary{}, errors.New("interactive session resume cancelled")
		}
	}
}

type selectionKey int

const (
	selectionIgnore selectionKey = iota
	selectionUp
	selectionDown
	selectionConfirm
	selectionCancel
)

func readSelectionKey(reader *bufio.Reader) (selectionKey, error) {
	value, err := reader.ReadByte()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return selectionIgnore, errors.New("interactive input closed")
		}
		return selectionIgnore, fmt.Errorf("read interactive input: %w", err)
	}
	switch value {
	case '\r', '\n':
		return selectionConfirm, nil
	case 'q', 'Q', 3:
		return selectionCancel, nil
	case 27:
		if reader.Buffered() < 2 {
			return selectionCancel, nil
		}
		first, err := reader.ReadByte()
		if err != nil || first != '[' {
			return selectionCancel, nil
		}
		second, err := reader.ReadByte()
		if err != nil {
			return selectionCancel, nil
		}
		if second == 'A' {
			return selectionUp, nil
		}
		if second == 'B' {
			return selectionDown, nil
		}
	}
	return selectionIgnore, nil
}

func makeRaw(input io.Reader) (func(), error) {
	file, ok := input.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return func() {}, nil
	}
	state, err := term.MakeRaw(int(file.Fd()))
	if err != nil {
		return nil, fmt.Errorf("enable interactive terminal input: %w", err)
	}
	return func() { _ = term.Restore(int(file.Fd()), state) }, nil
}

func displaySessionTime(value string, now time.Time) string {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return singleLine(value, 18)
	}
	parsed = parsed.In(now.Location())
	if sameCalendarDay(parsed, now) {
		return "Today " + parsed.Format("15:04")
	}
	if sameCalendarDay(parsed, now.AddDate(0, 0, -1)) {
		return "Yesterday " + parsed.Format("15:04")
	}
	if parsed.Year() == now.Year() {
		return parsed.Format("Jan 2 15:04")
	}
	return parsed.Format("Jan 2 2006")
}

func sameCalendarDay(left, right time.Time) bool {
	leftYear, leftMonth, leftDay := left.Date()
	rightYear, rightMonth, rightDay := right.Date()
	return leftYear == rightYear && leftMonth == rightMonth && leftDay == rightDay
}

func displayWorkspace(value string, max int) string {
	if value == "" {
		return "(workspace unknown)"
	}
	value = filepath.Clean(value)
	if home, err := os.UserHomeDir(); err == nil {
		if value == home {
			value = "~"
		} else if relative, err := filepath.Rel(home, value); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			value = filepath.Join("~", relative)
		}
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return "…" + string(runes[len(runes)-max+1:])
}
