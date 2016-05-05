package reboot

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.cmd.jujud.reboot")

type OpenRebootAPIFn func() (reboot.State, error)

// ExecuteReboot will wait for all running containers to stop, and
// then execute a shutdown or a reboot (based on the action param)
func ExecuteReboot(
	exec utils.CommandRunner,
	openRebootAPI OpenRebootAPIFn,
	delay time.Duration,
	action params.RebootAction,
	rebootOK <-chan error,
) error {
	if action == params.ShouldDoNothing {
		return nil
	}
	rebootState, err := openRebootAPI()
	if err != nil {
		return errors.Trace(err)
	}

	if err := <-rebootOK; err != nil {
		return errors.Trace(err)
	}

	cmd, args := buildRebootCommand(action, delay)
	if _, err := exec(cmd, args...); err != nil {
		return errors.Annotate(err, "cannot schedule reboot")
	}
	if err := rebootState.ClearReboot(); err != nil {
		return errors.Annotate(err, "cannot clear reboot request")
	}
	return nil
}
