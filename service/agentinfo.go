// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"strings"

	"github.com/juju/utils/shell"

	"github.com/juju/juju/agent/tools"
)

// TODO(ericsnow) Move this file to the agent package.

// AgentKind identifies the kind of agent.
type AgentKind string

// These are the agent kinds in juju.
const (
	AgentKindMachine AgentKind = "machine"
	AgentKindUnit    AgentKind = "unit"
)

var idOptions = map[AgentKind]string{
	AgentKindMachine: "--machine-id",
	AgentKindUnit:    "--unit-name",
}

// AgentInfo holds commonly used information about a juju agent.
type AgentInfo struct {
	name string

	// ID is the string that identifies the agent uniquely within
	// a juju environment.
	ID string

	// Kind is the kind of agent.
	Kind AgentKind

	// DataDir is the path to the agent's data dir.
	DataDir string

	// LogDir is the path to the agent's log dir.
	LogDir string
}

// NewAgentInfo composes a new AgentInfo for the given essentials.
func NewAgentInfo(kind AgentKind, id, dataDir, logDir string) AgentInfo {
	name := fmt.Sprintf("%s-%s", kind, strings.Replace(id, "/", "-", -1))

	info := AgentInfo{
		Kind:    kind,
		ID:      id,
		DataDir: dataDir,
		LogDir:  logDir,

		name: name,
	}
	return info
}

// NewMachineAgentInfo returns a new AgentInfo for a machine agent.
func NewMachineAgentInfo(id, dataDir, logDir string) AgentInfo {
	return NewAgentInfo(AgentKindMachine, id, dataDir, logDir)
}

// NewUnitAgentInfo returns a new AgentInfo for a unit agent.
func NewUnitAgentInfo(id, dataDir, logDir string) AgentInfo {
	return NewAgentInfo(AgentKindUnit, id, dataDir, logDir)
}

// ToolsDir returns the path to the agent's tools dir.
func (ai AgentInfo) ToolsDir(renderer shell.Renderer) string {
	return renderer.FromSlash(tools.ToolsDir(ai.DataDir, ai.name))
}

func (ai AgentInfo) jujud(renderer shell.Renderer) string {
	exeName := "jujud" + renderer.ExeSuffix()
	return renderer.Join(ai.ToolsDir(renderer), exeName)
}

func (ai AgentInfo) cmd(renderer shell.Renderer) string {
	// The agent always starts with debug turned on. The logger worker
	// will update this to the system logging environment as soon as
	// it starts.
	return strings.Join([]string{
		renderer.Quote(ai.jujud(renderer)),
		string(ai.Kind),
		"--data-dir", renderer.Quote(renderer.FromSlash(ai.DataDir)),
		idOptions[ai.Kind], ai.ID,
		"--debug",
	}, " ")
}

// execArgs returns an unquoted array of service arguments in case we need
// them later. One notable place where this is needed, is the windows service
// package, where CreateService correctly does quoting of executable path and
// individual arguments
func (ai AgentInfo) execArgs(renderer shell.Renderer) []string {
	return []string{
		string(ai.Kind),
		"--data-dir", renderer.FromSlash(ai.DataDir),
		idOptions[ai.Kind], ai.ID,
		"--debug",
	}
}

func (ai AgentInfo) logFile(renderer shell.Renderer) string {
	return renderer.Join(renderer.FromSlash(ai.LogDir), ai.name+".log")
}
