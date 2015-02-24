// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"path"

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
func MachineAgentConf(machineID, dataDir, logDir string) (common.Conf, string) {
	tag := names.NewMachineTag(machineID)
	tagStr := tag.String()

	toolsDir := tools.ToolsDir(dataDir, tagStr)

	cmd := path.Join(toolsDir, "jujud") +
		" machine" +
		" --data-dir " + utils.ShQuote(dataDir) +
		" --machine-id " + machineID +
		" --debug"

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
