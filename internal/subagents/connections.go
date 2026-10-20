package subagents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type Connection struct {
	ID              string `json:"id"`
	ClientInfo      any    `json:"client_info,omitempty"`
	PID             int    `json:"pid"`
	Cwd             string `json:"cwd"`
	Started         string `json:"started"`
	Protocol        string `json:"protocol"`
	Status          string `json:"status"`
	ChannelDelivery string `json:"channel_delivery"`
}

func (s *Server) recordConnection() (func(), error) {
	dir := filepath.Join(s.Home, ".local", "state", "agentx", "connections")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%d-%s.json", os.Getpid(), randomID()))
	s.connectionPath = path
	s.connectionID = "connection-" + randomID()
	data, _ := json.Marshal(Connection{ID: s.connectionID, PID: os.Getpid(), Cwd: s.Cwd, Started: time.Now().UTC().Format(time.RFC3339), Protocol: "2024-11-05", Status: "MCP server process running; host handshake not yet verified", ChannelDelivery: "unverified; host controls channel opt-in and feature/org gates"})
	if err := os.WriteFile(path, data, 0600); err != nil {
		return nil, err
	}
	return func() { os.Remove(path) }, nil
}

func Connections(home string) []Connection {
	result := []Connection{}
	paths, _ := filepath.Glob(filepath.Join(home, ".local", "state", "agentx", "connections", "*.json"))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var c Connection
		if json.Unmarshal(data, &c) != nil {
			continue
		}
		process, err := os.FindProcess(c.PID)
		if err != nil {
			continue
		}
		err = process.Signal(syscall.Signal(0))
		process.Release()
		if err == nil {
			result = append(result, c)
		}
	}
	return result
}

func (s *Server) connectionInitialized() {
	data, err := os.ReadFile(s.connectionPath)
	if err != nil {
		return
	}
	var c Connection
	if json.Unmarshal(data, &c) != nil {
		return
	}
	c.Status = "active MCP connection: host sent notifications/initialized"
	data, _ = json.Marshal(c)
	_ = os.WriteFile(s.connectionPath, data, 0600)
}

func (s *Server) connectionClient(raw json.RawMessage) {
	var request struct {
		ClientInfo any `json:"clientInfo"`
	}
	if json.Unmarshal(raw, &request) != nil {
		return
	}
	data, err := os.ReadFile(s.connectionPath)
	if err != nil {
		return
	}
	var c Connection
	if json.Unmarshal(data, &c) != nil {
		return
	}
	c.ClientInfo = request.ClientInfo
	data, _ = json.Marshal(c)
	_ = os.WriteFile(s.connectionPath, data, 0600)
}
