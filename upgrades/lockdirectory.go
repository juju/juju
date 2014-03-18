// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"
	"path"

	"launchpad.net/juju-core/utils/exec"
)

var ubuntuHome = "/home/ubuntu"

// Previously the lock directory was created when the uniter started. This
// allows serialization of all of the hook execution across units running on a
// single machine.  This lock directory is now also used but the juju-run
// command on the host machine.  juju-run also gets a lock on the hook
// execution fslock prior to execution.  However, the lock directory was owned
// by root, and the juju-run process was being executed by the ubuntu user, so
// we need to change the ownership of the lock directory to ubuntu:ubuntu.
// Also we need to make sure that this directory exists on machines with no
// units.
func ensureLockDirExistsAndUbuntuWritable(context Context) error {
	lockDir := path.Join(context.AgentConfig().DataDir(), "locks")
	// We only try to change ownership if there is an ubuntu user
	// defined, and we determine this by the existance of the home dir.
	command := fmt.Sprintf(""+
		"mkdir -p %s\n"+
		"[ -e %s ] && chown ubuntu:ubuntu %s\n",
		lockDir, ubuntuHome, lockDir)
	logger.Tracef("command: %s", command)
	result, err := exec.RunCommands(exec.RunParams{
		Commands: command,
	})
	if err != nil {
		return err
	}
	logger.Tracef("stdout: %s", result.Stdout)
	return nil
}
