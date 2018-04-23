// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/shell"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/service/common"
)

const (
	maxAgentFiles = 20000

	agentServiceTimeout = 300 // 5 minutes
)

// AgentConf returns the data that defines an init service config
// for the identified agent.
func AgentConf(info AgentInfo, renderer shell.Renderer) common.Conf {
	conf := common.Conf{
		Desc:          fmt.Sprintf("juju agent for %s", info.Name),
		ExecStart:     info.Cmd(renderer),
		Logfile:       info.LogFile(renderer),
		Env:           osenv.FeatureFlags(),
		Timeout:       agentServiceTimeout,
		ServiceBinary: info.Jujud(renderer),
		ServiceArgs:   info.ExecArgs(renderer),
	}

	switch info.Kind {
	case AgentKindMachine:
		conf.Limit = map[string]int{
			"nofile": maxAgentFiles,
		}
	case AgentKindUnit:
		conf.Desc = "juju unit agent for " + info.ID
	}

	return conf
}

//find all the agents available in the machine
func FindAgents(machineAgent *string, unitAgents *[]string, dataDir string) error {
	agentDir := filepath.Join(dataDir, "agents")
	dir, err := os.Open(agentDir)
	if err != nil {
		return errors.Annotate(err, "opening agents dir")
	}
	defer dir.Close()

	entries, err := dir.Readdir(-1)
	if err != nil {
		return errors.Annotate(err, "reading agents dir")
	}
	for _, info := range entries {
		name := info.Name()
		tag, err := names.ParseTag(name)
		if err != nil {
			continue
		}
		switch tag.Kind() {
		case names.MachineTagKind:
			*machineAgent = name
		case names.UnitTagKind:
			*unitAgents = append(*unitAgents, name)
		default:
			errors.Errorf("%s is not of type Machine nor Unit, ignoring", name)
		}
	}
	return nil
}

func CreateAgentConf(agentName string, dataDir string, series string) (_ common.Conf, err error) {
	defer func() {
		if err != nil {
			errors.Errorf("Failed create agent conf for %s: %s", agentName, err)
		}
	}()

	renderer, err := shell.NewRenderer("")
	if err != nil {
		return common.Conf{}, err
	}

	tag, err := names.ParseTag(agentName)
	if err != nil {
		return common.Conf{}, err
	}
	name := tag.Id()

	var kind AgentKind
	switch tag.Kind() {
	case names.MachineTagKind:
		kind = AgentKindMachine
	case names.UnitTagKind:
		kind = AgentKindUnit
	default:
		return common.Conf{}, errors.NewNotValid(nil, fmt.Sprintf("agent %q is neither a machine nor a unit", agentName))
	}

	info := NewAgentInfo(
		kind,
		name,
		dataDir,
		paths.MustSucceed(paths.LogDir(series)),
	)
	return AgentConf(info, renderer), nil
}
