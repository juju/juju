// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/service/initsystems"
)

// TODO(ericsnow) Incorporate all or part of this file into the agent package?

const (
	maxAgentFiles = 20000
	logSuffix     = ".log"
	agentPrefix   = "jujud-"
)

const (
	// TODO(ericsnow) Move this to the
	jujudName = "jujud"
)

var (
	agentOptions = map[string]string{
		"machine": "machine-id",
		"unit":    "unit-name",
	}
)

type agentServices interface {
	ListEnabled() ([]string, error)
}

// ListAgents builds a list of tags for all the machine and unit agents
// running on the host.
func ListAgents(services agentServices) ([]names.Tag, error) {
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

// TODO(ericsnow) Move AgentPaths to juju/paths, agent, or etc.?

// AgentPaths exposes the various paths that are associated with an
// agent (e.g. via the agent config).
type AgentPaths interface {
	DataDir() string
	LogDir() string
}

// TODO(ericsnow) Support explicitly setting the calculated values
// (e.g. executable) in AgentService?
// TODO(ericsnow) Refactor environs/cloudinit.MachineConfig relative
// to AgentService?

// AgentServiceSpec is the specification for the jujud service for a
// unit or machine agent. The kind is determined from the tag passed
// to NewAgentService.
type AgentServiceSpec struct {
	AgentPaths

	tag names.Tag
	env map[string]string

	initSystem string
	option     string
}

// NewAgentServiceSpec builds the specification for a new agent jujud
// service based on the provided information.
func NewAgentServiceSpec(tag names.Tag, paths AgentPaths, env map[string]string) (*AgentServiceSpec, error) {
	svc, err := newAgentServiceSpec(tag, paths, env)
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

func newAgentServiceSpec(tag names.Tag, paths AgentPaths, env map[string]string) (*AgentServiceSpec, error) {
	svc := &AgentServiceSpec{
		AgentPaths: paths,
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

// Name provides the agent's init system service name.
func (as AgentServiceSpec) Name() string {
	return agentPrefix + as.tag.String()
}

// ToolsDir composes path to the agent's tools dir from the AgentService
// and returns it.
func (as AgentServiceSpec) ToolsDir() string {
	return tools.ToolsDir(as.DataDir(), as.tag.String())
}

func (as AgentServiceSpec) executable() string {
	name := jujudName
	if as.initSystem == InitSystemWindows {
		name += ".exe"
	}
	executable := filepath.Join(as.ToolsDir(), name)
	return fromSlash(executable, as.initSystem)
}

func (as AgentServiceSpec) logfile() string {
	name := as.tag.String() + logSuffix
	return filepath.Join(as.LogDir(), name)
}

func (as AgentServiceSpec) command() string {
	// E.g. "jujud" machine --data-dir "..." --machine-id "0"
	command := fmt.Sprintf(`"%s" %s --data-dir "%s" --%s "%s"`,
		as.executable(),
		as.tag.Kind(),
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

// Conf returns the service config for the agent described by AgentService.
func (as AgentServiceSpec) Conf() Conf {
	cmd := as.command()

	normalConf := initsystems.Conf{
		Desc: fmt.Sprintf("juju agent for %s %s", as.tag.Kind(), as.tag.Id()),
		Cmd:  cmd,
	}
	conf := Conf{
		Conf: normalConf,
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
