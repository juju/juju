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
	dataPath string
	logPath  string

	name string

	// ID is the string that identifies the agent uniquely within
	// a juju environment.
	ID string

	// Kind is the kind of agent.
	Kind AgentKind
}

// NewAgentInfo composes a new AgentInfo for the given essentials.
func NewAgentInfo(kind AgentKind, dataPath, logPath, id string) AgentInfo {
	name := fmt.Sprintf("%s-%s", kind, strings.Replace(id, "/", "-", -1))

	info := AgentInfo{
		dataPath: dataPath,
		logPath:  logPath,
		Kind:     kind,
		ID:       id,
		name:     name,
	}
	return info
}

// NewMachineAgentInfo returns a new AgentInfo for a machine agent.
func NewMachineAgentInfo(dataPath, logPath, id string) AgentInfo {
	return NewAgentInfo(AgentKindMachine, dataPath, logPath, id)
}

// NewUnitAgentInfo returns a new AgentInfo for a unit agent.
func NewUnitAgentInfo(dataPath, logPath, id string) AgentInfo {
	return NewAgentInfo(AgentKindUnit, dataPath, logPath, id)
}

// ToolsDir returns the path to the agent's tools dir.
func (ai AgentInfo) ToolsDir(renderer shell.Renderer) string {
	return renderer.FromSlash(tools.ToolsDir(ai.dataPath, ai.name))
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
		"--data-dir", renderer.Quote(renderer.FromSlash(ai.dataPath)),
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
		"--data-dir", renderer.FromSlash(ai.dataPath),
		idOptions[ai.Kind], ai.ID,
		"--debug",
	}
}

func (ai AgentInfo) logFile(renderer shell.Renderer) string {
	return renderer.Join(renderer.FromSlash(ai.logPath), ai.name+".log")
}
