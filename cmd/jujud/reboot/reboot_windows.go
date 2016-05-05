// +build windows

package reboot

import (
	"fmt"
	"math"
	"time"

	"github.com/juju/juju/apiserver/params"
)

func buildRebootCommand(action params.RebootAction, delay time.Duration) (cmd string, args []string) {
	const shutdownCmd = "shutdown.exe"
	delayFlag := []string{"-t", fmt.Sprintf("%d", int64(math.Ceil(delay.Seconds())))}
	switch action {
	case params.ShouldReboot:
		return shutdownCmd, append(delayFlag, "-r")
	case params.ShouldShutdown:
		return shutdownCmd, append(delayFlag, "-s")
	}
	return "", nil
}
