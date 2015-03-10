// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"path"
	"runtime"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service/common"
)

const (
	maxAgentFiles = 20000

	agentServiceTimeout = 300 // 5 minutes
)

// TODO(ericsnow) Add agent.Info to handle all the agent-related data
// and pass it as *the* arg to the helpers.

func agentConf(kind, id, dataDir, logDir, os string) (common.Conf, string) {
	if os == "" {
		os = runtime.GOOS
	}
	name := kind + "-" + strings.Replace(id, "/", "-", -1)

	renderer, err := shell.NewRenderer(os)
	if err != nil {
		// This should not ever happen.
		panic(err)
	}
	toolsDir := renderer.FromSlash(tools.ToolsDir(dataDir, name))
	jujudPath := renderer.Join(toolsDir, "jujud") + renderer.ExeSuffix()

	idOption := "--machine-id"
	if kind == "unit" {
		idOption = "--unit-name"
	}

	cmd := strings.Join([]string{
		renderer.Quote(jujudPath),
		kind,
		"--data-dir", renderer.Quote(renderer.FromSlash(dataDir)),
		idOption, id,
		"--debug",
	}, " ")

	logFile := path.Join(logDir, name+".log")

	// The agent always starts with debug turned on. The logger worker
	// will update this to the system logging environment as soon as it starts.
	conf := common.Conf{
		Desc:      fmt.Sprintf("juju agent for %s", name),
		ExecStart: cmd,
		Logfile:   renderer.FromSlash(logFile),
		Env:       osenv.FeatureFlags(),
		//Limit: map[string]int{
		//	"nofile": maxAgentFiles,
		//},
		Timeout: agentServiceTimeout,
	}

	return conf, toolsDir
}

// MachineAgentConf returns the data that defines an init service config
// for the identified machine.
func MachineAgentConf(machineID, dataDir, logDir, os string) (common.Conf, string) {
	conf, toolsDir := agentConf("machine", machineID, dataDir, logDir, os)

	conf.Limit = map[string]int{
		"nofile": maxAgentFiles,
	}

	return conf, toolsDir
}

// UnitAgentConf returns the data that defines an init service config
// for the identified unit.
func UnitAgentConf(unitName, dataDir, logDir, os, containerType string) (common.Conf, string) {
	conf, toolsDir := agentConf("unit", unitName, dataDir, logDir, os)
	conf.Desc = "juju unit agent for " + unitName

	// TODO(thumper): 2013-09-02 bug 1219630
	// As much as I'd like to remove JujuContainerType now, it is still
	// needed as MAAS still needs it at this stage, and we can't fix
	// everything at once.
	envVars := map[string]string{
		osenv.JujuContainerTypeEnvKey: containerType,
	}
	osenv.MergeEnvironment(envVars, conf.Env)
	conf.Env = envVars

	return conf, toolsDir
}

// ShutdownAfterConf builds a service conf that will cause the host to
// shut down after the named service stops.
func ShutdownAfterConf(serviceName string) (common.Conf, error) {
	if serviceName == "" {
		return common.Conf{}, errors.New(`missing "after" service name`)
	}
	desc := "juju shutdown job"
	return shutdownAfterConf(serviceName, desc), nil
}

func shutdownAfterConf(serviceName, desc string) common.Conf {
	return common.Conf{
		Desc:         desc,
		Transient:    true,
		AfterStopped: serviceName,
		ExecStart:    "/sbin/shutdown -h now",
	}
}
