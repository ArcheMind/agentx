package sessions

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
)

func readJSONL(path string) ([]map[string]any, error) {
	var records []map[string]any
	err := scanJSONL(path, func(record map[string]any) bool {
		records = append(records, record)
		return false
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

// scanJSONL visits valid JSON objects until EOF or the visitor returns true.
// Native records can contain large tool outputs or images, so read one line at
// a time without Scanner's token-size limit. Invalid lines (including a partial
// final record in an active session) are skipped without hiding I/O errors.
func scanJSONL(path string, visit func(map[string]any) bool) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return err
		}
		var record map[string]any
		if json.Unmarshal(line, &record) == nil && visit(record) {
			return nil
		}
		if err == io.EOF {
			return nil
		}
	}
}
