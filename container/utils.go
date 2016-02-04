package container

import (
	"os/exec"
	"runtime"
)

var (
	runtimeGOOS = runtime.GOOS
)

func RunningInContainer() bool {
	if runtimeGOOS != "linux" {
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
