package sessions

import (
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// The on-disk contract is Antigravity CLI 1.5.8, SQLite user_version 1.
const agySummaryDDL = `CREATE TABLE conversation_summaries (conversation_id text PRIMARY KEY,title text NOT NULL DEFAULT '',preview text NOT NULL DEFAULT '',step_count integer NOT NULL DEFAULT 0,last_modified_time datetime NOT NULL,workspace_uris text NOT NULL,status text NOT NULL DEFAULT '',source text NOT NULL DEFAULT '',project_id text NOT NULL DEFAULT '',agent_name text NOT NULL DEFAULT '',parent_conversation_id text NOT NULL DEFAULT '',nesting_depth integer NOT NULL DEFAULT 0,battle_id text NOT NULL DEFAULT '',winning_conversation_id text NOT NULL DEFAULT '',not_fully_idle numeric NOT NULL DEFAULT false,killed numeric NOT NULL DEFAULT false,last_user_input_time datetime NOT NULL,last_user_input_step_index integer NOT NULL DEFAULT -1,app_data_dir text NOT NULL DEFAULT '',raw_summary blob,group_id text NOT NULL DEFAULT '')`

var agyDDL = []string{
	`CREATE TABLE trajectory_meta (trajectory_id text PRIMARY KEY,cascade_id text,trajectory_type integer,source integer)`,
	`CREATE TABLE steps (idx integer PRIMARY KEY,step_type integer NOT NULL DEFAULT 0,status integer NOT NULL DEFAULT 0,has_subtrajectory numeric NOT NULL DEFAULT false,metadata blob,error_details blob,permissions blob,task_details blob,render_info blob,step_payload blob,step_format integer NOT NULL DEFAULT 0)`,
	`CREATE TABLE gen_metadata (idx integer PRIMARY KEY,data blob,size integer NOT NULL DEFAULT 0)`,
	`CREATE TABLE executor_metadata (idx integer PRIMARY KEY,data blob)`,
	`CREATE TABLE parent_references (idx integer PRIMARY KEY,data blob)`,
	`CREATE TABLE trajectory_metadata_blob (id text PRIMARY KEY DEFAULT 'main',data blob)`,
	`CREATE TABLE battle_mode_infos (idx integer PRIMARY KEY,data blob)`,
	`CREATE INDEX idx_steps_status ON steps(status)`,
	`CREATE INDEX idx_steps_step_type ON steps(step_type)`,
}

type agyField struct {
	number  int
	value   []byte
	integer uint64
}
type agyProto []agyField

func agyDecode(data []byte) (agyProto, error) {
	var fields agyProto
	for len(data) > 0 {
		tag, n := binary.Uvarint(data)
		if n <= 0 || tag>>3 == 0 {
			return nil, fmt.Errorf("invalid agy protobuf tag")
		}
		data = data[n:]
		f := agyField{number: int(tag >> 3)}
		switch tag & 7 {
		case 0:
			v, n := binary.Uvarint(data)
			if n <= 0 {
				return nil, fmt.Errorf("invalid agy protobuf varint")
			}
			f.integer = v
			data = data[n:]
		case 1:
			if len(data) < 8 {
				return nil, fmt.Errorf("truncated agy protobuf fixed64")
			}
			data = data[8:]
		case 2:
			size, n := binary.Uvarint(data)
			if n <= 0 || size > uint64(len(data)-n) {
				return nil, fmt.Errorf("truncated agy protobuf bytes")
			}
			f.value = data[n : n+int(size)]
			data = data[n+int(size):]
		case 5:
			if len(data) < 4 {
				return nil, fmt.Errorf("truncated agy protobuf fixed32")
			}
			data = data[4:]
		default:
			return nil, fmt.Errorf("unsupported agy protobuf wire type %d", tag&7)
		}
		fields = append(fields, f)
	}
	return fields, nil
}
func (p agyProto) bytes(n int) []byte {
	for _, f := range p {
		if f.number == n {
			return f.value
		}
	}
	return nil
}
func (p agyProto) text(n int) string { return string(p.bytes(n)) }
func (p agyProto) integer(n int) uint64 {
	for _, f := range p {
		if f.number == n {
			return f.integer
		}
	}
	return 0
}
func agyBytes(n int, v []byte) []byte {
	b := binary.AppendUvarint(nil, uint64(n<<3|2))
	b = binary.AppendUvarint(b, uint64(len(v)))
	return append(b, v...)
}
func agyText(n int, v string) []byte {
	if v == "" {
		return nil
	}
	return agyBytes(n, []byte(v))
}
func agyInt(n int, v uint64) []byte {
	return binary.AppendUvarint(binary.AppendUvarint(nil, uint64(n<<3)), v)
}
func agyJoin(parts ...[]byte) []byte {
	var b []byte
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}
func agyTimestamp(value string) []byte {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t = time.Now().UTC()
	}
	return agyJoin(agyInt(1, uint64(t.Unix())), agyInt(2, uint64(t.Nanosecond())))
}
func agyTime(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	p, err := agyDecode(data)
	if err != nil {
		return "", err
	}
	return time.Unix(int64(p.integer(1)), int64(p.integer(2))).UTC().Format(time.RFC3339Nano), nil
}
func agyRoot(home string) string { return filepath.Join(home, ".gemini", "antigravity-cli") }
func agyOpen(path string, readonly bool) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	path = absolute
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := u.Query()
	if readonly {
		q.Set("mode", "ro")
	}
	q.Set("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}
func agyLocalResource(uri string) ([]byte, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") {
		return nil, fmt.Errorf("agy resource must be a local file URI: %q", uri)
	}
	path := u.Path
	if len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return os.ReadFile(filepath.FromSlash(path))
}
func agyWorkspace(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	p := u.Path
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

func loadAgy(home string) ([]Session, error) {
	files, err := filepath.Glob(filepath.Join(agyRoot(home), "conversations", "*.db"))
	if err != nil {
		return nil, err
	}
	var details []Session
	for _, path := range files {
		d, err := ParseAgySessionFile(path)
		if err != nil {
			return nil, err
		}
		details = append(details, d)
	}
	summaryPath := filepath.Join(agyRoot(home), "conversation_summaries.db")
	if _, err := os.Stat(summaryPath); os.IsNotExist(err) {
		return details, nil
	}
	db, err := agyOpen(summaryPath, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT conversation_id,title FROM conversation_summaries`)
	if err != nil {
		return nil, fmt.Errorf("read agy summaries: %w", err)
	}
	defer rows.Close()
	titles := map[string]string{}
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, err
		}
		titles[id] = title
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range details {
		if t := titles[details[i].ID]; t != "" {
			details[i].Title = t
		}
	}
	return details, nil
}

// ParseAgySessionFile reads SQLite directly, including committed WAL records.
func ParseAgySessionFile(path string) (Session, error) {
	d := newDetail("agy", path)
	db, err := agyOpen(path, true)
	if err != nil {
		return d, err
	}
	defer db.Close()
	var version int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return d, err
	}
	if version != 1 {
		return d, fmt.Errorf("unsupported agy SQLite user_version %d", version)
	}
	if err = db.QueryRow(`SELECT cascade_id FROM trajectory_meta LIMIT 1`).Scan(&d.ID); err != nil {
		return d, fmt.Errorf("read agy trajectory identity: %w", err)
	}
	var meta []byte
	err = db.QueryRow(`SELECT data FROM trajectory_metadata_blob WHERE id='main'`).Scan(&meta)
	if err != nil && err != sql.ErrNoRows {
		return d, err
	}
	if len(meta) > 0 {
		p, e := agyDecode(meta)
		if e != nil {
			return d, e
		}
		d.Workspace = agyWorkspace(p.text(7))
		d.StartedAt, e = agyTime(p.bytes(2))
		if e != nil {
			return d, e
		}
		d.IsSubagent = p.text(5) != "" || p.integer(17) > 0
	}
	rows, err := db.Query(`SELECT idx,step_payload,step_format FROM steps ORDER BY idx`)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var idx, format int
		var raw []byte
		if err := rows.Scan(&idx, &raw, &format); err != nil {
			return d, err
		}
		if format != 0 {
			return d, fmt.Errorf("unsupported agy step format %d", format)
		}
		p, e := agyDecode(raw)
		if e != nil {
			return d, fmt.Errorf("agy step %d: %w", idx, e)
		}
		m, e := parseAgyStep(p, idx)
		if e != nil {
			return d, fmt.Errorf("agy step %d: %w", idx, e)
		}
		if m.Role == "" {
			continue
		}
		if len(d.Messages) > 0 {
			m.ParentID = d.Messages[len(d.Messages)-1].ID
		}
		d.Messages = append(d.Messages, m)
		updateTimeBounds(&d, m.Timestamp)
	}
	if err := rows.Err(); err != nil {
		return d, err
	}
	d, _ = finishDetail(d)
	return d, nil
}
func agyTool(data []byte) (ContentBlock, error) {
	p, err := agyDecode(data)
	b := ContentBlock{Type: "tool_use", ID: p.text(1), Name: p.text(2)}
	if err != nil {
		return b, err
	}
	if p.text(3) != "" {
		if err = json.Unmarshal(p.bytes(3), &b.Input); err != nil {
			return b, fmt.Errorf("invalid agy tool arguments: %w", err)
		}
	}
	return b, nil
}
func parseAgyStep(p agyProto, idx int) (Message, error) {
	m := Message{ID: fmt.Sprintf("agy-%d", idx)}
	meta, err := agyDecode(p.bytes(5))
	if err != nil {
		return m, err
	}
	m.Timestamp, err = agyTime(meta.bytes(1))
	if err != nil {
		return m, err
	}
	switch p.integer(1) {
	case 14:
		u, err := agyDecode(p.bytes(19))
		if err != nil {
			return m, err
		}
		m.Role = "user"
		if text := u.text(2); text != "" {
			m.Content = append(m.Content, ContentBlock{Type: "text", Text: text})
		}
		for _, f := range u {
			if f.number != 9 {
				continue
			}
			media, e := agyDecode(f.value)
			if e != nil {
				return m, e
			}
			data := media.bytes(2)
			if len(data) == 0 && media.text(5) != "" {
				data, e = agyLocalResource(media.text(5))
				if e != nil {
					return m, e
				}
			}
			if len(data) == 0 {
				return m, fmt.Errorf("agy media has no inline data or local resource")
			}
			m.Content = append(m.Content, ContentBlock{Type: "image", Source: map[string]any{"type": "base64", "media_type": media.text(1), "data": base64.StdEncoding.EncodeToString(data)}})
		}
	case 15:
		a, err := agyDecode(p.bytes(20))
		if err != nil {
			return m, err
		}
		m.Role = "assistant"
		sig := a.text(4)
		if raw := a.bytes(14); len(raw) > 0 {
			sig = "agy:bytes:" + base64.StdEncoding.EncodeToString(raw)
		}
		if a.text(3) != "" || sig != "" {
			m.Content = append(m.Content, ContentBlock{Type: "thinking", Text: a.text(3), Signature: sig})
		}
		if a.text(1) != "" {
			m.Content = append(m.Content, ContentBlock{Type: "text", Text: a.text(1)})
		}
		for _, f := range a {
			if f.number == 7 {
				b, e := agyTool(f.value)
				if e != nil {
					return m, e
				}
				m.Content = append(m.Content, b)
			}
		}
	default:
		if len(meta.bytes(4)) == 0 {
			if raw := p.bytes(114); len(raw) > 0 {
				s, e := agyDecode(raw)
				if e != nil {
					return m, e
				}
				if s.text(1) != "" {
					m.Role = "system"
					m.Content = []ContentBlock{{Type: "text", Text: s.text(1)}}
				}
			}
			return m, nil
		}
		call, e := agyTool(meta.bytes(4))
		if e != nil {
			return m, e
		}
		m.Role = "tool"
		m.ToolUseID = call.ID
		m.ToolName = call.Name
		m.IsError = len(p.bytes(31)) > 0 || p.integer(4) == 7
		switch {
		case len(p.bytes(140)) > 0:
			g, e := agyDecode(p.bytes(140))
			if e != nil {
				return m, e
			}
			r, e := agyDecode(g.bytes(2))
			if e != nil {
				return m, e
			}
			text := r.text(1)
			if r.text(7) != "" {
				data, e := agyLocalResource(r.text(7))
				if e != nil {
					return m, e
				}
				text = string(data)
			}
			m.Result = text
		case len(p.bytes(47)) > 0:
			r, e := agyDecode(p.bytes(47))
			if e != nil {
				return m, e
			}
			text := r.text(3)
			if r.text(8) != "" {
				data, e := agyLocalResource(r.text(8))
				if e != nil {
					return m, e
				}
				text = string(data)
			}
			m.Result = text
		case len(p.bytes(14)) > 0:
			r, e := agyDecode(p.bytes(14))
			if e != nil {
				return m, e
			}
			m.Result = firstNonEmpty(r.text(9), r.text(4))
		case len(p.bytes(28)) > 0:
			r, e := agyDecode(p.bytes(28))
			if e != nil {
				return m, e
			}
			output, e := agyDecode(r.bytes(21))
			if e != nil {
				return m, e
			}
			text := output.text(1)
			if text == "" {
				snapshot, e := agyDecode(r.bytes(26))
				if e != nil {
					return m, e
				}
				text = snapshot.text(1)
			}
			if text == "" {
				text = r.text(4) + r.text(5)
			}
			m.Result = text
			m.IsError = m.IsError || r.integer(6) != 0
		case len(p.bytes(15)) > 0:
			r, e := agyDecode(p.bytes(15))
			if e != nil {
				return m, e
			}
			var names []string
			for _, f := range r {
				if f.number == 2 {
					names = append(names, string(f.value))
				}
				if f.number == 3 {
					entry, e := agyDecode(f.value)
					if e != nil {
						return m, e
					}
					names = append(names, entry.text(1))
				}
			}
			m.Result = strings.Join(names, "\n")
			m.IsError = m.IsError || r.integer(4) != 0
		default:
			return m, fmt.Errorf("unsupported agy tool result step type %d", p.integer(1))
		}
	}
	return m, nil
}

// SerializeAgy returns native Step protobuf payloads without source-store metadata.
func SerializeAgy(detail Session) ([][]byte, error) {
	var steps [][]byte
	for _, m := range replayMessages(detail, "agy") {
		stamp := agyTimestamp(m.Timestamp)
		meta := agyJoin(agyBytes(1, stamp), agyBytes(6, stamp), agyBytes(7, stamp), agyBytes(8, stamp), agyBytes(32, stamp))
		var body []byte
		var typ, key int
		switch m.Role {
		case "user":
			typ, key = 14, 19
			body = agyText(2, textContent(m.Content))
			for _, b := range m.Content {
				if b.Type != "image" {
					continue
				}
				source, ok := b.Source.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("agy image source must be an object")
				}
				data, err := base64.StdEncoding.DecodeString(stringValue(source["data"]))
				if err != nil {
					return nil, err
				}
				mime := stringValue(source["media_type"])
				if len(data) == 0 || mime == "" {
					return nil, fmt.Errorf("agy image requires inline base64 data and media_type")
				}
				body = append(body, agyBytes(9, agyJoin(agyText(1, mime), agyBytes(2, data)))...)
			}
		case "assistant":
			// Separate steps preserve tagged thought boundaries when foreign thinking becomes readable text.
			var pending []byte
			flush := func() {
				if len(pending) > 0 {
					steps = append(steps, agyJoin(agyInt(1, 15), agyInt(4, 3), agyBytes(5, meta), agyBytes(20, pending)))
					pending = nil
				}
			}
			for _, b := range m.Content {
				switch b.Type {
				case "text":
					if len(pending) > 0 {
						flush()
					}
					pending = agyText(1, b.Text)
				case "thinking":
					flush()
					if b.Signature == "" {
						if b.Text != "" {
							pending = agyText(1, thinkTagText(b.Text))
							flush()
						}
						continue
					}
					pending = agyText(3, b.Text)
					if strings.HasPrefix(b.Signature, "agy:bytes:") {
						raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(b.Signature, "agy:bytes:"))
						if err != nil {
							return nil, err
						}
						pending = append(pending, agyBytes(14, raw)...)
					} else {
						pending = append(pending, agyText(4, b.Signature)...)
					}
					flush()
				case "tool_use":
					args, err := json.Marshal(b.Input)
					if err != nil {
						return nil, err
					}
					pending = append(pending, agyBytes(7, agyJoin(agyText(1, b.ID), agyText(2, b.Name), agyText(3, string(args))))...)
				}
			}
			flush()
			continue
		case "tool":
			typ, key = 132, 140
			body = agyBytes(2, agyText(1, resultText(m.Result)))
			call := agyJoin(agyText(1, m.ToolUseID), agyText(2, m.ToolName), agyText(3, "{}"))
			meta = append(meta, agyBytes(4, call)...)
		case "system":
			typ, key = 101, 114
			body = agyText(1, textContent(m.Content))
		default:
			return nil, fmt.Errorf("unsupported agy message role %q", m.Role)
		}
		status := uint64(3)
		var errorDetails []byte
		if m.IsError {
			status = 7
			errorDetails = agyBytes(31, agyText(1, resultText(m.Result)))
		}
		steps = append(steps, agyJoin(agyInt(1, uint64(typ)), agyInt(4, status), agyBytes(5, meta), errorDetails, agyBytes(key, body)))
	}
	return steps, nil
}

// WriteAgyDatabaseFile creates a standalone native database at a new path.
func WriteAgyDatabaseFile(detail Session, path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(path)
		}
	}()
	db, err := agyOpen(path, false)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, ddl := range agyDDL {
		if _, err = tx.Exec(ddl); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`PRAGMA user_version=1`); err != nil {
		return err
	}
	if detail.ID == "" {
		detail.ID = newUUID()
	}
	workspace := detail.Workspace
	if workspace == "" {
		workspace, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return err
	}
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(workspace)}).String()
	metadata := agyJoin(agyBytes(1, agyText(1, uri)), agyBytes(2, agyTimestamp(detail.StartedAt)), agyText(3, newUUID()), agyText(6, detail.ID), agyText(7, uri), agyText(18, "default-cli-project"))
	if _, err = tx.Exec(`INSERT INTO trajectory_meta VALUES (?,?,4,17)`, newUUID(), detail.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO trajectory_metadata_blob VALUES ('main',?)`, metadata); err != nil {
		return err
	}
	steps, err := SerializeAgy(detail)
	if err != nil {
		return err
	}
	for i, step := range steps {
		p, e := agyDecode(step)
		if e != nil {
			return e
		}
		if _, err = tx.Exec(`INSERT INTO steps(idx,step_type,status,has_subtrajectory,metadata,error_details,step_payload,step_format) VALUES (?,?,?,0,?,?,?,0)`, i, p.integer(1), p.integer(4), p.bytes(5), p.bytes(31), step); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	ok = true
	return nil
}

func WriteAgySessionFile(detail Session, home string) (string, error) {
	id := newUUID()
	detail.ID = id
	root := agyRoot(home)
	dir := filepath.Join(root, "conversations")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, id+".db")
	if err := WriteAgyDatabaseFile(detail, path); err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(path)
		}
	}()
	db, err := agyOpen(filepath.Join(root, "conversation_summaries.db"), false)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err = db.Exec(strings.Replace(agySummaryDDL, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", 1)); err != nil {
		return "", err
	}
	workspace := detail.Workspace
	if workspace == "" {
		workspace, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	uris, _ := json.Marshal([]string{(&url.URL{Scheme: "file", Path: filepath.ToSlash(workspace)}).String()})
	steps, err := SerializeAgy(detail)
	if err != nil {
		return "", err
	}
	summary := SummaryOf(detail)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	lastUser := -1
	for i, step := range steps {
		p, _ := agyDecode(step)
		if p.integer(1) == 14 {
			lastUser = i
		}
	}
	_, err = db.Exec(`INSERT INTO conversation_summaries(conversation_id,title,preview,step_count,last_modified_time,workspace_uris,last_user_input_time,last_user_input_step_index,project_id) VALUES (?,?,?,?,?,?,?,?,?)`, id, firstNonEmpty(detail.Title, summary.FirstUserMessage), summary.LatestUserMessage, len(steps), now, string(uris), now, lastUser, "default-cli-project")
	if err != nil {
		return "", err
	}
	ok = true
	return id, nil
}
