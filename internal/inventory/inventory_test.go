package inventory

import (
	"strings"
	"testing"
)

func TestSourceIdentityAndSelectionF01F02F04F10(t *testing.T) {
	input := []Source{
		{Scope: "user", Path: "/user/private", Status: "ok", Resources: []Resource{{Name: "alpha", Path: "/user/private/alpha"}}},
		{Scope: "project", Path: "/project/private", Status: "ok", Resources: []Resource{{Name: "alpha", Path: "/project/private/alpha"}}},
		{Scope: "project", Path: "/project/common", Preferred: true, Status: "ok", Agents: []string{"claude"}, Resources: []Resource{{Name: "alpha", Path: "/project/common/alpha", Agents: []string{"claude"}}, {Name: "beta", Path: "/project/common/beta", Agents: []string{"claude"}}}},
		{Scope: "project", Path: "/project/common", Preferred: true, Status: "ok", Agents: []string{"codex"}, Resources: []Resource{{Name: "alpha", Path: "/project/common/alpha", Agents: []string{"codex"}}}},
		{Scope: "user", Path: "/user/common", Preferred: true, Status: "ok", Resources: []Resource{{Name: "delta", Path: "/user/common/delta"}}},
	}
	result := Normalize(input)
	if len(result.Sources) != 4 || result.Sources[0].Path != "/project/common" || result.Sources[2].Path != "/user/common" {
		t.Fatalf("sources=%+v", result)
	}
	first := result.Sources[0]
	if len(first.Resources) != 2 || len(first.Resources[0].Agents) != 2 || len(first.Resources[1].Agents) != 1 {
		t.Fatalf("aggregation=%+v", first)
	}
	if _, err := result.Show("alpha", Selection{}); err == nil || !strings.Contains(err.Error(), "--source") {
		t.Fatalf("ambiguity=%v", err)
	}
	detail, err := result.Show("alpha", Selection{Scope: "user", Source: "/user/private"})
	if err != nil || detail.Source.Scope != "user" {
		t.Fatalf("selection=%+v %v", detail, err)
	}
}
func TestEvidenceF08(t *testing.T) {
	cases := []struct {
		input               Evidence
		connection, session string
	}{
		{Configured(), "unconfirmed", "unconfirmed"},
		{Evidence{Configuration: "discovered", Connection: "connected", ConnectionEvidence: "probe 123", Session: "loaded"}, "connected", "unconfirmed"},
		{Evidence{Configuration: "discovered", Connection: "connected", ConnectionEvidence: "probe 456", Session: "loaded", SessionID: "session-7", SessionEvidence: "session-7 tool registration"}, "connected", "loaded"},
	}
	for _, c := range cases {
		got := c.input.Verified()
		if got.Connection != c.connection || got.Session != c.session {
			t.Fatalf("evidence=%+v", got)
		}
	}
}

func TestEvidenceAggregationDoesNotLoseOrPromoteSessions(t *testing.T) {
	configured := Source{Scope: "project", Path: "/common", Status: "ok", Resources: []Resource{{Name: "alpha", Path: "/common/alpha", Evidence: Configured()}}}
	connected := configured
	connected.Resources = []Resource{{Name: "alpha", Path: "/common/alpha", Evidence: Evidence{Configuration: "discovered", Connection: "connected", ConnectionEvidence: "probe-a", Session: "loaded", SessionID: "session-a", SessionEvidence: "registration-a"}}}
	result := Normalize([]Source{configured, connected})
	got := result.Sources[0].Resources[0]
	if got.Evidence.Connection != "connected" || got.Evidence.SessionID != "session-a" {
		t.Fatalf("later evidence lost: %+v", got)
	}
	another := configured
	another.Resources = []Resource{{Name: "alpha", Path: "/common/alpha", Evidence: Evidence{Configuration: "discovered", Connection: "connected", ConnectionEvidence: "probe-b", Session: "loaded", SessionID: "session-b", SessionEvidence: "registration-b"}}}
	result = Normalize([]Source{configured, connected, another})
	got = result.Sources[0].Resources[0]
	if got.Evidence.Session != "unconfirmed" || got.Evidence.SessionID != "" || len(got.Observations) != 3 {
		t.Fatalf("distinct sessions promoted or lost: %+v", got)
	}
}
