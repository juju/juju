// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"fmt"
	"path"

	"github.com/juju/utils"

	"github.com/juju/juju/service/common"
)

// MachineAgentSystemdService returns a systemd service conf for a machine
// agent based on the tag and machineID passed to it.
func MachineAgentSystemdService(name, toolsDir, dataDir, tag, machineID string, env map[string]string) *Service {
	// The machine agent will always start with DEBUG on; the logging level
	// being meant to updated by the logger worker as soon as it turns on.
	conf := common.Conf{
		Desc: fmt.Sprintf("juju %s agent", tag),
		Cmd: path.Join(toolsDir, "jujud") +
			" machine " +
			" --data-dir " + utils.ShQuote(dataDir) +
			" --machine-id " + machineID +
			" --debug",
		Env: env,
	}

	return NewService(name, conf)
}
