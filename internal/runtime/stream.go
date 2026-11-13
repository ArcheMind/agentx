package runtime

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// Stream is a long-lived external process with line-oriented stdin/stdout.
type Stream struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan string
	stderr *lockedBuffer
	debug  bool
	log    io.Writer

	waitOnce sync.Once
	waitErr  error

	writeMu sync.Mutex
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// StreamStarter starts a long-lived line-oriented process in its own process group.
type StreamStarter interface {
	StartStream(plan CommandPlan) (*Stream, error)
}

func (r ExecRunner) StartStream(plan CommandPlan) (*Stream, error) {
	cmd := exec.Command(plan.Executable, plan.Args...)
	cmd.Dir = plan.Cwd
	cmd.Env = mergeEnvironment(os.Environ(), plan.Env, plan.UnsetEnv)
	// cmd.Cancel needs a command context, so the stream sets only the process group here
	// and kills the group itself.
	configureOwnProcessGroup(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open external command %s stdin: %w", plan.Executable, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open external command %s stdout: %w", plan.Executable, err)
	}
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start external command %s: %w", plan.Executable, err)
	}
	s := &Stream{cmd: cmd, stdin: stdin, lines: make(chan string, 64), stderr: stderr, debug: r.Debug, log: r.Log}
	if s.debug {
		data, _ := json.Marshal(map[string]any{"event": "stream_start", "input": redactPlan(plan), "pid": cmd.Process.Pid})
		fmt.Fprintln(s.log, string(data))
	}
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			s.logLine("out", line)
			s.lines <- line
		}
		close(s.lines)
	}()
	return s, nil
}

func (s *Stream) logLine(direction, line string) {
	if !s.debug {
		return
	}
	data, _ := json.Marshal(map[string]any{"event": "stream_line", "direction": direction, "line": RedactSecrets(line)})
	fmt.Fprintln(s.log, string(data))
}

// Lines yields stdout lines; the channel closes on process EOF.
func (s *Stream) Lines() <-chan string { return s.lines }

func (s *Stream) WriteLine(line string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.logLine("in", line)
	_, err := io.WriteString(s.stdin, line+"\n")
	return err
}

func (s *Stream) CloseStdin() error { return s.stdin.Close() }

// Kill terminates the whole process group.
func (s *Stream) Kill() { killProcessTree(s.cmd, s.debug, s.log) }

func (s *Stream) Wait() error {
	s.waitOnce.Do(func() {
		s.waitErr = s.cmd.Wait()
		if s.debug {
			data, _ := json.Marshal(map[string]any{"event": "stream_exit", "error_output": RedactSecrets(s.stderr.String()), "error": fmt.Sprint(s.waitErr)})
			fmt.Fprintln(s.log, string(data))
		}
	})
	return s.waitErr
}

func (s *Stream) Stderr() string { return s.stderr.String() }
