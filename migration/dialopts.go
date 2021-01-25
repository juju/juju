// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"time"

	"github.com/juju/juju/api"
)

// ControllerDialOpts returns dial parameters suitable for connecting
// from the source controller to the target controller during model
// migrations.
// Except for the inclusion of RetryDelay the options mirror what is used
// by the APICaller for logins.
func ControllerDialOpts() api.DialOpts {
	return api.DialOpts{
		DialTimeout:         3 * time.Second,
		DialAddressInterval: 200 * time.Millisecond,
		Timeout:             time.Minute,
		RetryDelay:          100 * time.Millisecond,
	}
}
