// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"time"

	"github.com/juju/juju/api"
)

// ControllerDialOpts returns dial parameters suitable for connecting
// from the source controller to the target controller during model
// migrations. The total attempt time can't be too long because the
// areas of the code which make these connections need to be
// interruptable but a number of retries is useful to deal with short
// lived issues.
func ControllerDialOpts() api.DialOpts {
	return api.DialOpts{
		DialAddressInterval: 50 * time.Millisecond,
		Timeout:             1 * time.Second,
		RetryDelay:          100 * time.Millisecond,
	}
}
