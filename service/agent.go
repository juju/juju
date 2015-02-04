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
			// The name was not a tag (e.g. juju-mongod).
			continue
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// AgentPaths exposes the various paths that are associated with an
// agent (e.g. via the agent config).
type AgentPaths interface {
	DataDir() string
	LogDir() string
}

// AgentServiceSpec is the specification for the jujud service for a
// unit or machine agent. The kind is determined from the tag passed
// to NewAgentService.
type AgentServiceSpec struct {
	AgentPaths

	Env        map[string]string
	tag        names.Tag
	initSystem string
	option     string
}

// NewAgentServiceSpec builds the specification for a new agent jujud
// service based on the provided information.
func NewAgentServiceSpec(tag names.Tag, paths AgentPaths, initSystem string) (*AgentServiceSpec, error) {
	svc := &AgentServiceSpec{
		AgentPaths: paths,
		tag:        tag,
		initSystem: initSystem,
	}

	option, ok := agentOptions[svc.tag.Kind()]
	if !ok {
		return nil, errors.NotSupportedf("tag %v", svc.tag)
	}
	svc.option = option

	return svc, nil
}

// DiscoverAgentServiceSpec builds the specification for a new agent
// jujud service based on the provided information.
func DiscoverAgentServiceSpec(tag names.Tag, paths AgentPaths) (*AgentServiceSpec, error) {
	init, err := discoverInitSystem()
	if err != nil {
		return nil, errors.Trace(err)
	}

	svc, err := NewAgentServiceSpec(tag, paths, init)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return svc, nil
}

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
	conf.Env = as.Env
	if as.tag.Kind() == "machine" {
		conf.Limit = map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxAgentFiles, maxAgentFiles),
		}
	}

	return conf
}

// NewAgentService builds a new Service for the juju agent
// identified by the provided information and returns it.
func NewAgentService(tag names.Tag, paths AgentPaths, env map[string]string, services services) (*Service, error) {
	spec, err := NewAgentServiceSpec(tag, paths, services.InitSystem())
	if err != nil {
		return nil, errors.Trace(err)
	}
	spec.Env = env

	svc := NewService(spec.Name(), spec.Conf(), services)
	return svc, nil
}
