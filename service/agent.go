// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"path"

	"github.com/juju/utils"

	"github.com/juju/juju/service/common"
)

// TODO(ericsnow) Move this whole file over to the agent package?

const (
	maxAgentFiles = 20000
	logSuffix     = ".log"
)

// TODO(ericsnow) Move executables and the exe* consts to the proper
// spot (agent?).

const (
	exeWindows = "jujud.exe"
	exeDefault = "jujud"
)

var (
	executables = map[string]string{
		InitSystemWindows: exeWindows,
		InitSystemUpstart: exeDefault,
	}
)

// TODO(ericsnow) Move AgentPaths to juju/paths, agent, or etc.?

type AgentPaths struct {
	DataDir string
	LogDir  string
}

func (as AgentPaths) ToolsDir() string {
	return tools.ToolsDir(as.DataDir, as.Tag)
}

// TODO(ericsnow) Support explicitly setting the calculated values
// (e.g. executable) in AgentService?
// TODO(ericsnow) Refactor environs/cloudinit.MachineConfig relative
// to AgentService?

type AgentService struct {
	AgentPaths

	Name      string
	Tag       string
	MachineID string
	Env       map[string]string

	initSystem string // CloudInitInstallCommands sets this.
}

// TODO(ericsnow) Support discovering init system on remote host.

// TODO(ericsnow) Is guarding against unset fields really necessary.
// We could add a Validate method; or for the less efficient one-off
// case, we could add an error return on the dynamic attr methods.

func (as AgentService) init() (string, error) {
	if as.initSystem != "" {
		return as.initSystem, nil
	}

	init, err := discoverInitSystem()
	if err != nil {
		return "", errors.Trace(err)
	}

	as.initSystem = init
	return init, nil
}

func (as AgentService) executable() string {
	name := exeDefault

	init, err := as.init()
	if err == nil {
		// TODO(ericsnow) Is it safe enough to use the default
		// executable when the init system is unknown?
		name = executables[init]
	}

	return filepath.Join(as.ToolsDir(), name)
}

func (as AgentService) logfile() string {
	name := as.Tag + logSuffix
	return path.Join(as.LogDir, name)
}

// Conf returns the init config for the agent described by AgentService.
func (as AgentService) Conf() *common.Conf {

	init, err := as.init()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var conf *common.Conf
	switch init {
	case InitSystemWindows:
		conf = as.confWindows()
	default:
		conf = as.confLinux()
	}

	return conf, nil
}

// TODO(ericsnow) Move confWindows to service/windows and confLinux
// to service/common?

func (as AgentService) confWindows() *common.Conf {
	// This method must convert slashes to backslashes.

	serviceString := fmt.Sprintf(`"%s" machine --data-dir "%s" --machine-id "%s" --debug`,
		w.renderer.FromSlash(as.executable()),
		w.renderer.FromSlash(as.DataDir),
		as.MachineID)

	cmd := []string{
		fmt.Sprintf(`New-Service -Credential $jujuCreds -Name '%s' -DisplayName 'Jujud machine agent' '%s'`, name, serviceString),
		fmt.Sprintf(`cmd.exe /C sc config %s start=delayed-auto`, name),
		fmt.Sprintf(`Start-Service %s`, name),
	}
	return cmd
}

func (as AgentService) confLinux() *common.Conf {
	// The machine agent always starts with debug turned on. The logger
	// worker will update this to the system logging environment as soon
	// as it starts.
	conf := common.Conf{
		Desc: fmt.Sprintf("juju %s agent", as.Tag),
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxAgentFiles, maxAgentFiles),
		},
		Cmd: as.executable() +
			" machine" +
			" --data-dir " + utils.ShQuote(as.DataDir) +
			" --machine-id " + as.MachineID +
			" --debug",
		Out: as.logfile(),
		Env: as.Env,
	}
	return &conf
}
