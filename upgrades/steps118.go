// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"
	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/utils/exec"
)

// stepsFor118 returns upgrade steps to upgrade to a Juju 1.18 deployment.
func stepsFor118(context Context) []UpgradeStep {
	return []UpgradeStep{
	// Nothing yet.
	}
}

// Ensure that the lock dir exists and change the ownership of the lock dir
// itself to ubuntu:ubuntu from root:root so the juju-run command run as the
// ubuntu user is able to get access to the hook execution lock (like the
// uniter itself does.)
func ensureLockDirExistsAndUbuntuWritable() {
	// Pretty sure we want the agent Config in the context...
	var agentConfig agent.Config // TODO: get from context

	lockDir := path.Join(agentConfig.DataDir(), "locks")
	// We only try to change ownership if there is an ubuntu user
	// defined, and we determine this by the existance of the home dir.
	command := fmt.Sprintf(""+
		"mkdir -p %s\n"+
		"[ -e /home/ubuntu ] && chown ubuntu:ubuntu %s",
		lockDir, lockDir)
	result, err := exec.RunCommands(exec.RunParams{
		Commands:   command,
		WorkingDir: ProxyDirectory,
	})
	if err != nil {
		return err
	}
	if result.Code != 0 {
		logger.Errorf("failed writing new proxy values: \n%s\n%s", result.Stdout, result.Stderr)
	}
}
