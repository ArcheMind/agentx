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

func (a App) recentSessionGroups(includeSubagents bool) ([]sessionGroup, error) {
	items, err := a.Sessions.RecentGroups("", recentSessionsPerProvider, includeSubagents)
	if err != nil {
		return nil, err
	}
	return []sessionGroup{
		{Heading: "Current workspace", Items: items.Current},
		{Heading: "Global", Items: items.Global},
	}, nil
}

type contentRow struct {
	text      string
	itemIndex int // selectable item index; -1 for structural rows
	kind      contentRowKind
	provider  string
	updated   string
	workspace string
	title     string
}

type contentRowKind int

const (
	contentRowPlain contentRowKind = iota
	contentRowHeading
	contentRowEmpty
	contentRowSession
)

func buildContentRows(groups []sessionGroup, now time.Time) []contentRow {
	var rows []contentRow
	itemIndex := 0
	for _, group := range groups {
		rows = append(rows, contentRow{text: "", itemIndex: -1})
		rows = append(rows, contentRow{text: group.Heading, itemIndex: -1, kind: contentRowHeading})
		if len(group.Items) == 0 {
			rows = append(rows, contentRow{text: "  No recent sessions", itemIndex: -1, kind: contentRowEmpty})
			continue
		}
		for _, item := range group.Items {
			title := item.Title
			if title == "" {
				title = item.ID
			}
			updated := displaySessionTime(item.UpdatedAt, now)
			if duration := displaySessionDuration(item.StartedAt, item.UpdatedAt); duration != "" {
				updated += " " + duration
			}
			if group.Heading == "Global" {
				text := fmt.Sprintf("%-8s  %-25s  %-20s  %s", item.Provider, updated, displayWorkspace(item.Workspace, 20), singleLine(title, 35))
				rows = append(rows, contentRow{
					text: text, itemIndex: itemIndex, kind: contentRowSession,
					provider: item.Provider, updated: updated, workspace: displayWorkspace(item.Workspace, 20), title: singleLine(title, 35),
				})
			} else {
				text := fmt.Sprintf("%-8s  %-25s  %s", item.Provider, updated, singleLine(title, 57))
				rows = append(rows, contentRow{
					text: text, itemIndex: itemIndex, kind: contentRowSession,
					provider: item.Provider, updated: updated, title: singleLine(title, 57),
				})
			}
			itemIndex++
		}
	}
	return rows
}

func selectedRowRange(rows []contentRow, selected int) (int, int) {
	for i, r := range rows {
		if r.itemIndex == selected {
			return i, i
		}
	}
	return 0, 0
}

func terminalHeight(input io.Reader) int {
	file, ok := input.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return 0
	}
	_, height, err := term.GetSize(int(file.Fd()))
	if err != nil {
		return 0
	}
	return height
}

func (a App) chooseSession(reader *bufio.Reader, groups []sessionGroup) (sessions.Summary, error) {
	items := make([]sessions.Summary, 0)
	for _, group := range groups {
		items = append(items, group.Items...)
	}

	now := time.Now()
	rows := buildContentRows(groups, now)
	selected, err := a.chooseInteractive(reader, "Choose a recent session:", rows, len(items), errors.New("no recent sessions found in the current workspace or globally"))
	if err != nil {
		return sessions.Summary{}, err
	}
	return items[selected], nil
}

func (a App) chooseOptions(reader *bufio.Reader, title string, options []string) (int, error) {
	rows := make([]contentRow, len(options))
	for index, option := range options {
		rows[index] = contentRow{text: option, itemIndex: index, provider: option}
	}
	return a.chooseInteractive(reader, title, rows, len(options), errors.New("no options available"))
}

func (a App) chooseInteractive(reader *bufio.Reader, title string, rows []contentRow, itemCount int, emptyErr error) (int, error) {
	selected := 0
	viewStart := 0
	lineCount := 0
	termH := a.termHeight
	if termH == 0 {
		termH = terminalHeight(a.Stdin)
	}

	const headerLines = 2

	render := func(clear bool) {
		if clear {
			fmt.Fprintf(a.Stdout, "\x1b[%dA\r\x1b[J", lineCount)
		}
		lineCount = headerLines
		fmt.Fprintf(a.Stdout, "%s\r\n", colorize(a.Color, ansiBold, title))
		fmt.Fprintf(a.Stdout, "%s\r\n", colorize(a.Color, ansiDim, "Use ↑/↓ or j/k to move, Enter to select, q to cancel."))

		visStart := 0
		visEnd := len(rows)

		if termH > 0 && len(rows) > termH-headerLines {
			height := termH - headerLines - 2
			if height < 1 {
				height = 1
			}
			selFirst, selLast := selectedRowRange(rows, selected)
			if selFirst < viewStart {
				viewStart = selFirst
			}
			if selLast >= viewStart+height {
				viewStart = selLast - height + 1
			}
			if viewStart < 0 {
				viewStart = 0
			}
			if viewStart+height > len(rows) {
				viewStart = len(rows) - height
				if viewStart < 0 {
					viewStart = 0
				}
			}
			visStart = viewStart
			visEnd = viewStart + height
			if visEnd > len(rows) {
				visEnd = len(rows)
			}
		}

		hiddenAbove := 0
		for _, r := range rows[:visStart] {
			if r.itemIndex >= 0 {
				hiddenAbove++
			}
		}
		hiddenBelow := 0
		for _, r := range rows[visEnd:] {
			if r.itemIndex >= 0 {
				hiddenBelow++
			}
		}

		if hiddenAbove > 0 {
			fmt.Fprintf(a.Stdout, "%s\r\n", colorize(a.Color, ansiDim, fmt.Sprintf("  ↑ %d more", hiddenAbove)))
			lineCount++
		}

		for _, row := range rows[visStart:visEnd] {
			if row.itemIndex >= 0 {
				marker := "  "
				if row.itemIndex == selected {
					marker = "> "
				}
				fmt.Fprintf(a.Stdout, "%s%s\r\n", styledMarker(marker, row, a.Color), styledContentRow(row, row.itemIndex == selected, a.Color))
			} else {
				fmt.Fprintf(a.Stdout, "%s\r\n", styledContentRow(row, false, a.Color))
			}
			lineCount++
		}

		if hiddenBelow > 0 {
			fmt.Fprintf(a.Stdout, "%s\r\n", colorize(a.Color, ansiDim, fmt.Sprintf("  ↓ %d more", hiddenBelow)))
			lineCount++
		}
	}

	render(false)
	if itemCount == 0 {
		return 0, emptyErr
	}

	restore, err := makeRaw(a.Stdin)
	if err != nil {
		return 0, err
	}
	defer restore()

	for {
		key, err := readSelectionKey(reader)
		if err != nil {
			return 0, err
		}
		switch key {
		case selectionUp:
			if selected > 0 {
				selected--
				render(true)
			}
		case selectionDown:
			if selected < itemCount-1 {
				selected++
				render(true)
			}
		case selectionConfirm:
			fmt.Fprint(a.Stdout, "\r\n")
			return selected, nil
		case selectionCancel:
			fmt.Fprint(a.Stdout, "\r\n")
			return 0, errors.New("interactive session resume cancelled")
		}
	}
}

func styledMarker(marker string, row contentRow, color bool) string {
	if marker == "  " {
		return marker
	}
	style := agentColor(row.provider)
	if style == "" {
		style = ansiCyan
	}
	return colorize(color, style+ansiBold, marker)
}

func styledContentRow(row contentRow, selected, color bool) string {
	switch row.kind {
	case contentRowHeading:
		return colorize(color, ansiBold, row.text)
	case contentRowEmpty:
		return colorize(color, ansiDim, row.text)
	case contentRowSession:
		provider := colorize(color, agentColor(row.provider), fmt.Sprintf("%-8s", row.provider))
		updated := colorize(color, ansiDim, fmt.Sprintf("%-25s", row.updated))
		titleStyle := ""
		if selected {
			titleStyle = ansiBold
		}
		title := colorize(color && titleStyle != "", titleStyle, row.title)
		if row.workspace == "" {
			return fmt.Sprintf("%s  %s  %s", provider, updated, title)
		}
		workspace := colorize(color, ansiDim, fmt.Sprintf("%-20s", row.workspace))
		return fmt.Sprintf("%s  %s  %s  %s", provider, updated, workspace, title)
	default:
		style := agentColor(row.provider)
		if selected {
			style += ansiBold
		}
		return colorize(color && style != "", style, row.text)
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
	case 'k', 'K':
		return selectionUp, nil
	case 'j', 'J':
		return selectionDown, nil
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
		return singleLine(value, 15)
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

func displaySessionDuration(startedAt, updatedAt string) string {
	started, startErr := time.Parse(time.RFC3339Nano, startedAt)
	updated, updateErr := time.Parse(time.RFC3339Nano, updatedAt)
	if startErr != nil || updateErr != nil || updated.Before(started) {
		return ""
	}
	duration := updated.Sub(started)
	if duration < time.Minute {
		return fmt.Sprintf("(%ds)", int(duration/time.Second))
	}
	if duration < time.Hour {
		return fmt.Sprintf("(%dm)", int(duration/time.Minute))
	}
	if duration < 24*time.Hour {
		return fmt.Sprintf("(%dh %02dm)", int(duration/time.Hour), int(duration/time.Minute)%60)
	}
	days := int(duration / (24 * time.Hour))
	if days > 999 {
		return "(999d+)"
	}
	return fmt.Sprintf("(%dd %02dh)", days, int(duration/time.Hour)%24)
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
