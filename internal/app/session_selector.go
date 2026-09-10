package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ArcheMind/agentx/internal/sessions"
	"golang.org/x/term"
)

const recentSessionsPerProvider = 10

type sessionGroup struct {
	Heading string
	Items   []sessions.Summary
}

func (a App) recentSessionGroups() ([]sessionGroup, error) {
	current, err := a.Sessions.List(sessions.ListOptions{Limit: recentSessionsPerProvider, Sort: "date"})
	if err != nil {
		return nil, err
	}
	all, err := a.Sessions.List(sessions.ListOptions{All: true, Limit: 0, Sort: "date"})
	if err != nil {
		return nil, err
	}

	currentKeys := make(map[string]bool, len(current))
	for _, item := range current {
		currentKeys[sessionKey(item)] = true
	}
	counts := make(map[string]int)
	global := make([]sessions.Summary, 0)
	for _, item := range all {
		if currentKeys[sessionKey(item)] || counts[item.Provider] >= recentSessionsPerProvider {
			continue
		}
		counts[item.Provider]++
		global = append(global, item)
	}

	return []sessionGroup{
		{Heading: "Current workspace", Items: current},
		{Heading: "Global", Items: global},
	}, nil
}

func sessionKey(item sessions.Summary) string {
	return item.Provider + "\x00" + item.ID
}

func (a App) chooseSession(reader *bufio.Reader, groups []sessionGroup) (sessions.Summary, error) {
	items := make([]sessions.Summary, 0)
	for _, group := range groups {
		items = append(items, group.Items...)
	}

	selected := 0
	lineCount := 0
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
				fmt.Fprintf(a.Stdout, "%s%-10s  %-20s  %s\r\n", marker, item.Provider, item.UpdatedAt, singleLine(title, 72))
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
