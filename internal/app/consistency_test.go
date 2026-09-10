package app

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/drivers"
	"github.com/ArcheMind/agentx/internal/runtime"
)

func TestCrossLayerConsistency(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	var help bytes.Buffer
	App{Stdout: &help}.printHelp()
	for _, command := range []string{
		"agent list",
		"agent which <agent>",
		"agent install <agent>",
		"agent models <agent>",
		"agent run <agent>",
		"auth login <agent>",
		"auth status <agent>",
		"auth logout <agent>",
		"session <providers|list|info|resume>",
	} {
		if !strings.Contains(help.String(), command) {
			t.Errorf("help does not advertise %q", command)
		}
		if !strings.Contains(string(readme), command) {
			t.Errorf("README does not advertise %q", command)
		}
	}

	for _, agent := range drivers.NewRegistry().All() {
		assertCapability(t, agent, runtime.CapabilityLaunch, agent.Launch != nil)
		assertCapability(t, agent, runtime.CapabilityModelList, agent.Models != nil && !isUnsupportedModels(agent.Models))
		assertCapability(t, agent, runtime.CapabilityAuthLogin, agent.Auth != nil)
		assertCapability(t, agent, runtime.CapabilityAuthStatus, agent.Auth != nil && agent.Auth.SupportsStatus())
		assertCapability(t, agent, runtime.CapabilityAuthLogout, agent.Auth != nil && agent.Auth.SupportsLogout())
		if agent.Install == nil {
			t.Errorf("%s is advertised as an Agent but has no install driver", agent.ID)
		}
	}
}

func assertCapability(t *testing.T, agent runtime.Agent, capability runtime.Capability, supported bool) {
	t.Helper()
	found := false
	for _, candidate := range agent.Capabilities {
		if candidate == capability {
			found = true
			break
		}
	}
	if found != supported {
		t.Errorf("%s capability %s = %t, driver support = %t", agent.ID, capability, found, supported)
	}
}

func isUnsupportedModels(driver runtime.ModelDriver) bool {
	_, unsupported := driver.(drivers.UnsupportedModels)
	return unsupported
}
