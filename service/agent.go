// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"path"
	"runtime"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/cloudinit"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service/common"
)

const (
	maxAgentFiles = 20000

	agentServiceTimeout = 300 // 5 minutes
)

// TODO(ericsnow) Factor out the common parts between the two helpers.
// TODO(ericsnow) Add agent.Info to handle all the agent-related data
// and pass it as *the* arg to the helpers.

// MachineAgentConf returns the data that defines an init service config
// for the identified machine.
func MachineAgentConf(machineID, dataDir, logDir, os string) (common.Conf, string) {
	machineName := "machine-" + strings.Replace(machineID, "/", "-", -1)

	var renderer cloudinit.Renderer = &cloudinit.UbuntuRenderer{}
	jujudSuffix := ""
	shquote := utils.ShQuote
	if os == "windows" {
		renderer = &cloudinit.WindowsRenderer{}
		jujudSuffix = ".exe"
		shquote = func(path string) string { return `"` + path + `"` }
	}
	toolsDir := renderer.FromSlash(tools.ToolsDir(dataDir, machineName))
	jujudPath := renderer.PathJoin(toolsDir, "jujud") + jujudSuffix

	cmd := strings.Join([]string{
		shquote(jujudPath),
		"machine",
		"--data-dir", shquote(renderer.FromSlash(dataDir)),
		"--machine-id", machineID, // TODO(ericsnow) double-quote on windows?
		"--debug",
	}, " ")

	logFile := path.Join(logDir, machineName+".log")

	// The machine agent always starts with debug turned on.  The logger worker
	// will update this to the system logging environment as soon as it starts.
	conf := common.Conf{
		Desc:      fmt.Sprintf("juju agent for %s", machineName),
		ExecStart: cmd,
		Logfile:   renderer.FromSlash(logFile),
		Env:       osenv.FeatureFlags(),
		Limit: map[string]int{
			"nofile": maxAgentFiles,
		},
		Timeout: agentServiceTimeout,
	}

	return conf, toolsDir
}

// UnitAgentConf returns the data that defines an init service config
// for the identified unit.
func UnitAgentConf(unitName, dataDir, logDir, os, containerType string) (common.Conf, string) {
	if os == "" {
		os = runtime.GOOS
	}
	var renderer cloudinit.Renderer = &cloudinit.UbuntuRenderer{}
	if os == "windows" {
		renderer = &cloudinit.WindowsRenderer{}
	}
	unitID := "unit-" + strings.Replace(unitName, "/", "-", -1)

	toolsDir := tools.ToolsDir(dataDir, unitID)
	jujudPath := path.Join(toolsDir, "jujud")
	if os == "windows" {
		jujudPath += ".exe"
	}

	cmd := strings.Join([]string{
		renderer.FromSlash(jujudPath),
		"unit",
		"--data-dir", utils.ShQuote(renderer.FromSlash(dataDir)),
		"--unit-name", unitName,
		"--debug",
	}, " ")

	logFile := path.Join(logDir, unitID+".log")

	// TODO(thumper): 2013-09-02 bug 1219630
	// As much as I'd like to remove JujuContainerType now, it is still
	// needed as MAAS still needs it at this stage, and we can't fix
	// everything at once.
	envVars := map[string]string{
		osenv.JujuContainerTypeEnvKey: containerType,
	}
	osenv.MergeEnvironment(envVars, osenv.FeatureFlags())

	// The machine agent always starts with debug turned on.  The logger worker
	// will update this to the system logging environment as soon as it starts.
	conf := common.Conf{
		Desc:      fmt.Sprintf("juju unit agent for %s", unitName),
		ExecStart: cmd,
		Logfile:   renderer.FromSlash(logFile),
		Env:       envVars,
		Timeout:   agentServiceTimeout,
	}

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
