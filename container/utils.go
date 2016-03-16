// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"os/exec"
)

func RunningInContainer() bool {
	/* running-in-container is in init-scripts-helpers, and is smart enough
	* to ask both systemd and upstart whether or not they know if the task
	* is running in a container.
	 */
	cmd := exec.Command("running-in-container")
	return cmd.Run() == nil
}
