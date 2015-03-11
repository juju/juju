// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"strings"

	"github.com/juju/juju/agent/tools"
)

// TODO(ericsnow) Move to the agent package.

type AgentKind string

const (
	AgentKindMachine AgentKind = "machine"
	AgentKindUnit    AgentKind = "unit"
)

var idOptions = map[AgentKind]string{
	AgentKindMachine: "--machine-id",
	AgentKindUnit:    "--unit-name",
}

type AgentInfo struct {
	Kind    AgentKind
	ID      string
	DataDir string
	LogDir  string

	name string
}

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

func NewMachineAgentInfo(id, dataDir, logDir string) AgentInfo {
	return NewAgentInfo(AgentKindMachine, id, dataDir, logDir)
}

func NewUnitAgentInfo(id, dataDir, logDir string) AgentInfo {
	return NewAgentInfo(AgentKindUnit, id, dataDir, logDir)
}

func (ai AgentInfo) toolsDir(renderer renderer) string {
	return renderer.FromSlash(tools.ToolsDir(ai.DataDir, ai.name))
}

func (ai AgentInfo) jujud(renderer renderer) string {
	exeName := "jujud" + renderer.exeSuffix
	return renderer.PathJoin(ai.toolsDir(renderer), exeName)
}

func (ai AgentInfo) cmd(renderer renderer) string {
	// The agent always starts with debug turned on. The logger worker
	// will update this to the system logging environment as soon as
	// it starts.
	return strings.Join([]string{
		renderer.shquote(ai.jujud(renderer)),
		string(ai.Kind),
		"--data-dir", renderer.shquote(renderer.FromSlash(ai.DataDir)),
		idOptions[ai.Kind], ai.ID,
		"--debug",
	}, " ")
}

func (ai AgentInfo) logFile(renderer renderer) string {
	return renderer.PathJoin(renderer.FromSlash(ai.LogDir), ai.name+".log")
}
