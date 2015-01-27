// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/service/common"
)

// TODO(ericsnow) Move this whole file over to the agent package?

const (
	maxAgentFiles = 20000
	logSuffix     = ".log"
	agentPrefix   = "jujud-"
)

// TODO(ericsnow) Move executables and the exe* consts to the proper
// spot (agent?). This is currently sort of addressed in the juju/names
// package, but that doesn't accommodate remote init systems.

const (
	exeWindows = "jujud.exe"
	exeDefault = "jujud"
)

var (
	agentExecutables = map[string]string{
		InitSystemWindows: exeWindows,
		InitSystemUpstart: exeDefault,
	}

	agentOptions = map[string]string{
		"machine": "machine-id",
		"unit":    "unit-name",
	}
)

func ListAgents(services *Services) ([]names.Tag, error) {
	enabled, err := services.ListEnabled()
	if err != nil {
		return nil, errors.Trace(err)
	}

	start := len(agentPrefix)
	var tags []names.Tag
	for _, name := range enabled {
		if !strings.HasPrefix(name, agentPrefix) {
			continue
		}
		tag, err := names.ParseTag(name[start:])
		if err != nil {
			// TODO(ericsnow) Fail here?
			continue
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// TODO(ericsnow) Move agentPaths to juju/paths, agent, or etc.?

type agentPaths interface {
	DataDir() string
	LogDir() string
}

// TODO(ericsnow) Support explicitly setting the calculated values
// (e.g. executable) in AgentService?
// TODO(ericsnow) Refactor environs/cloudinit.MachineConfig relative
// to AgentService?

type AgentService struct {
	agentPaths

	tag names.Tag
	env map[string]string

	initSystem string
	option     string
}

func NewAgentService(tag names.Tag, paths agentPaths, env map[string]string) (*AgentService, error) {
	svc, err := newAgentService(tag, paths, env)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(ericsnow) This will not work for remote systems.
	init, err := discoverInitSystem()
	if err != nil {
		return nil, errors.Trace(err)
	}
	svc.initSystem = init

	return svc, nil
}

func newAgentService(tag names.Tag, paths agentPaths, env map[string]string) (*AgentService, error) {
	svc := &AgentService{
		agentPaths: paths,
		tag:        tag,
		env:        env,
	}

	option, ok := agentOptions[svc.tag.Kind()]
	if !ok {
		return nil, errors.NotSupportedf("tag %v", svc.tag)
	}
	svc.option = option

	return svc, nil
}

// TODO(ericsnow) Support discovering init system on remote host.

// TODO(ericsnow) Is guarding against unset fields really necessary.
// We could add a Validate method; or for the less efficient one-off
// case, we could add an error return on the dynamic attr methods.

func (as AgentService) Name() string {
	return agentPrefix + as.tag.String()
}

func (as AgentService) ToolsDir() string {
	return tools.ToolsDir(as.DataDir(), as.tag.String())
}

func (as AgentService) executable() string {
	// TODO(ericsnow) Just use juju/names.Jujud for local?
	name := agentExecutables[as.initSystem]
	executable := filepath.Join(as.ToolsDir(), name)
	return fromSlash(executable, as.initSystem)
}

func (as AgentService) logfile() string {
	name := as.tag.String() + logSuffix
	return filepath.Join(as.LogDir(), name)
}

func (as AgentService) command() string {
	// E.g. "jujud" machine --data-dir "..." --machine-id "0"
	command := fmt.Sprintf(`"%s" %s --data-dir "%s" --%s "%s"`,
		as.executable(),
		as.tag.String(),
		fromSlash(as.DataDir(), as.initSystem),
		as.option,
		as.tag.Id(),
	)

	// The agent always starts with debug turned on. The logger
	// worker will update this to the system logging environment as soon
	// as it starts.
	command += " --debug"

	return command
}

// Conf returns the init config for the agent described by AgentService.
func (as AgentService) Conf() common.Conf {
	cmd := as.command()

	conf := common.Conf{
		Desc: fmt.Sprintf("juju agent for %s", as.tag.String()),
		Cmd:  cmd,
	}

	if as.initSystem == InitSystemWindows {
		// For Windows we do not set Out, Env, or Limit.
		return conf
	}

	// Populate non-Windows settings.
	conf.Out = as.logfile()
	conf.Env = as.env
	if as.tag.Kind() == "machine" {
		conf.Limit = map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxAgentFiles, maxAgentFiles),
		}
	}

	return conf
}
