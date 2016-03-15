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
	 * to ask both systemd and upstart whether or not they know if the task
	 * is running in a container.
	 */
	cmd := exec.Command("running-in-container")
	return cmd.Run() == nil
}

func ContainersSupported() bool {
	return !RunningInContainer()
}
