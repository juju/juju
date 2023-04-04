// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"os/exec"
	"runtime"
)

var RunningInContainer = func() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	/* running-in-container is in init-scripts-helpers, and is smart enough
	 * to ask systemd whether it knows if the task is running in a container.
	 */
	cmd := exec.Command("running-in-container")
	return cmd.Run() == nil
}

func ContainersSupported() bool {
	return !RunningInContainer()
}
