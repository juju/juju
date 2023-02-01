// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"github.com/juju/errors"
)

// These errors are returned by various specific workers in the hope that they
// will have some specific effect on the top-level agent running that worker.
//
// It should be clear that they don't belong here, and certainly shouldn't be
// used as they are today: e.g. a uniter has *no fricking idea* whether its
// host agent should shut down. A uniter can return ErrUnitDead, and its host
// might need to respond to that, perhaps by returning an error specific to
// *its* host; depending on these values punching right through N layers (but
// only when we want them to!) is kinda terrible.

const (
	// ErrRebootMachine indicates that the machine the agent is running on
	// should be rebooted.
	ErrRebootMachine = errors.ConstError("machine needs to reboot")

	// ErrRestartAgent indicates that the agent should be
	// restarted.
	ErrRestartAgent = errors.ConstError("agent should be restarted")

	// ErrShutdownMachine indicates the machine the agent is running on should
	// be shutdown.
	ErrShutdownMachine = errors.ConstError("machine needs to shutdown")

	// ErrTerminateAgent indicates the agent should be terminated.
	ErrTerminateAgent = errors.ConstError("agent should be terminated")
)
