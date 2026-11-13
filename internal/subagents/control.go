package subagents

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Control protocol: one line-JSON request and one response per connection on the daemon's
// unix socket.
type controlRequest struct {
	Op       string `json:"op"` // send | stop | status | message
	Message  string `json:"message,omitempty"`
	Delivery string `json:"delivery,omitempty"` // steer | queued
}

type controlResponse struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	Disposition string `json:"disposition,omitempty"` // send: steered | queued | started
	State       string `json:"state,omitempty"`       // status: running | idle
	Seq         int    `json:"seq,omitempty"`
	HadWork     bool   `json:"had_work,omitempty"`  // stop: anything was running or queued
	Cancelled   bool   `json:"cancelled,omitempty"` // stop: cancellation confirmed
	Status      string `json:"status,omitempty"`    // stop: terminal status of the last run
}

func controlCall(socket string, request controlRequest) (controlResponse, error) {
	var response controlResponse
	conn, err := net.DialTimeout("unix", socket, 2*time.Second)
	if err != nil {
		return response, err
	}
	defer conn.Close()
	data, err := json.Marshal(request)
	if err != nil {
		return response, err
	}
	// Stops wait for native process-group death; everything else is immediate.
	deadline := 15 * time.Second
	if request.Op == "stop" {
		deadline = 60 * time.Second
	}
	_ = conn.SetDeadline(time.Now().Add(deadline))
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return response, err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return response, fmt.Errorf("daemon closed before responding: %w", err)
	}
	if err := json.Unmarshal(line, &response); err != nil {
		return response, fmt.Errorf("invalid daemon response: %w", err)
	}
	if !response.OK {
		return response, fmt.Errorf("%s", response.Error)
	}
	return response, nil
}
