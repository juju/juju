// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"fmt"
	"path"

	"github.com/juju/utils"

	"github.com/juju/juju/service/common"
)

const (
	maxAgentFiles = 20000
)

// MachineAgentUpstartService returns the upstart config for a machine agent
// based on the tag and machineId passed in.
func MachineAgentUpstartService(name, toolsDir, dataDir, logDir, tag, machineId string, env map[string]string) *Service {
	logFile := path.Join(logDir, tag+".log")
	// The machine agent always starts with debug turned on.  The logger worker
	// will update this to the system logging environment as soon as it starts.
	conf := common.Conf{
		Desc: fmt.Sprintf("juju %s agent", tag),
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxAgentFiles, maxAgentFiles),
		},
		Cmd: path.Join(toolsDir, "jujud") +
			" machine" +
			" --data-dir " + utils.ShQuote(dataDir) +
			" --machine-id " + machineId +
			" --debug",
		Out: logFile,
		Env: env,
	}
	svc := NewService(name, conf)
	return svc
}
