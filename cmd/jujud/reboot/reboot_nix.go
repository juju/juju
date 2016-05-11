// +build !windows

package reboot

import (
	"fmt"
	"math"
	"time"

	"github.com/juju/juju/apiserver/params"
)

func buildRebootCommand(action params.RebootAction, delay time.Duration) (cmd string, args []string) {
	const shutdownCmd = "shutdown"
	delayArg := fmt.Sprintf("+%d", int64(math.Ceil(delay.Minutes())))
	switch action {
	case params.ShouldReboot:
		return shutdownCmd, []string{"-r", delayArg}
	case params.ShouldShutdown:
		return shutdownCmd, []string{"-h", delayArg}
	}

	return "", nil
}
