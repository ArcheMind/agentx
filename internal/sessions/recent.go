package sessions

import (
	"fmt"
	"os"
)

type RecentGroups struct {
	Current []Summary
	Global  []Summary
}

func (s Service) RecentGroups(workspace string, limit int, includeSubagents bool) (RecentGroups, error) {
	if limit <= 0 {
		return RecentGroups{}, fmt.Errorf("recent session limit must be positive")
	}
	if workspace == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return RecentGroups{}, fmt.Errorf("read working directory: %w", err)
		}
	}
	workspace = cleanPath(workspace)

	entries, err := s.catalog("")
	if err != nil {
		return RecentGroups{}, err
	}

	result := RecentGroups{Current: []Summary{}, Global: []Summary{}}
	currentCounts := make(map[string]int)
	globalCounts := make(map[string]int)
	for _, entry := range entries {
		item := entry.Summary
		if !entry.visible("", includeSubagents) {
			continue
		}
		if matchesWorkspaceSummary(item, workspace) {
			if currentCounts[item.Agent] < limit {
				currentCounts[item.Agent]++
				result.Current = append(result.Current, item)
			}
			continue
		}
		if globalCounts[item.Agent] < limit {
			globalCounts[item.Agent]++
			result.Global = append(result.Global, item)
		}
	}
	for _, items := range [][]Summary{result.Current, result.Global} {
		for i := range items {
			item, err := sessionEntry{Summary: items[i]}.summary()
			if err != nil {
				return RecentGroups{}, err
			}
			items[i] = item
		}
	}
	return result, nil
}
