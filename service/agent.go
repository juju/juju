// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"path"
	"runtime"
	"strings"

	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service/common"
)

const (
	maxAgentFiles = 20000
)

// MachineAgentConf returns the data that defines an init service config
// for the identified machine.
func MachineAgentConf(machineID, dataDir, logDir, os string) (common.Conf, string) {
	tag := names.NewMachineTag(machineID)
	tagStr := tag.String()

	toolsDir := tools.ToolsDir(dataDir, tagStr)
	jujudPath := path.Join(toolsDir, "jujud")
	if os == "windows" {
		jujudPath += ".exe"
	}

	cmd := strings.Join([]string{
		jujudPath,
		"machine",
		"--data-dir", utils.ShQuote(dataDir),
		"--machine-id", machineID,
		"--debug",
	}, " ")

	logFile := path.Join(logDir, tagStr+".log")

	// The machine agent always starts with debug turned on.  The logger worker
	// will update this to the system logging environment as soon as it starts.
	conf := common.Conf{
		Desc:      fmt.Sprintf("juju agent for %s", tag),
		ExecStart: cmd,
		Out:       logFile,
		Env:       osenv.FeatureFlags(),
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxAgentFiles, maxAgentFiles),
		},
	}

	return conf, toolsDir
}

// UnitAgentConf returns the data that defines an init service config
// for the identified unit.
func UnitAgentConf(unitName, dataDir, logDir, os, containerType string) (common.Conf, string) {
	if os == "" {
		os = runtime.GOOS
	}

	tag := names.NewUnitTag(unitName)
	tagStr := tag.String()

	toolsDir := tools.ToolsDir(dataDir, tagStr)
	jujudPath := path.Join(toolsDir, "jujud")
	if os == "windows" {
		jujudPath += ".exe"
	}

	cmd := strings.Join([]string{
		jujudPath,
		"unit",
		"--data-dir", utils.ShQuote(dataDir),
		"--unit-name", unitName,
		"--debug",
	}, " ")

	logFile := path.Join(logDir, tagStr+".log")

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
		Out:       logFile,
		Env:       envVars,
	}

	return conf, toolsDir
}
