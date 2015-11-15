// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"github.com/juju/errors"
)

var ErrTerminateAgent = errors.New("agent should be terminated")
var ErrRebootMachine = errors.New("machine needs to reboot")
var ErrShutdownMachine = errors.New("machine needs to shutdown")
