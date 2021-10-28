// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"fmt"

	"github.com/juju/juju/core/params"
)

// scheduleAction will do a reboot or shutdown after given number of seconds
// this function executes the operating system's reboot binary with appropriate
// parameters to schedule the reboot
// If action is params.ShouldDoNothing, it will return immediately.
// NOTE: On Windows the shutdown command is async
func scheduleAction(action params.RebootAction, after int) error {
	if action == params.ShouldDoNothing {
		return nil
	}
	args := []string{"shutdown.exe", "-f"}
	switch action {
	case params.ShouldReboot:
		args = append(args, "-r")
	case params.ShouldShutdown:
		args = append(args, "-s")
	}
	args = append(args, "-t", fmt.Sprintf("%d", after))

	return runCommand(args)
}
