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
var (
	ErrTerminateAgent  = errors.New("agent should be terminated")
	ErrRebootMachine   = errors.New("machine needs to reboot")
	ErrShutdownMachine = errors.New("machine needs to shutdown")
)
