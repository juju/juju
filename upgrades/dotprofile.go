// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"

	"launchpad.net/juju-core/utils/exec"
)

// As of the middle of the 1.17 cycle, the proxy settings are written out to
// /home/ubuntu/.juju-proxy both by cloud-init and the machine environ worker.
// An older version of juju that has been upgraded will get the proxy settings
// written out to the .juju-proxy file, but the .profile for the ubuntu user
// wouldn't have been updated to source this file.
//
// This upgrade step is to add the line to source the file if it is missing
// from the file.
func ensureUbuntuDotProfileSourcesProxyFile(context Context) error {
	// We look to see if the proxy line is there already as the manual
	// provider may have had it aleady. The ubuntu user may not exist
	// (local provider only).
	command := fmt.Sprintf(""+
		`([ ! -e %s/.profile ] || grep -q '.juju-proxy' %s/.profile) || `+
		`printf '\n# Added by juju\n[ -f "$HOME/.juju-proxy" ] && . "$HOME/.juju-proxy"\n' >> %s/.profile`,
		ubuntuHome, ubuntuHome, ubuntuHome)
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
