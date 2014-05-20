// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package audit records auditable events
package audit

import (
	"fmt"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/state"
)

var logger = loggo.GetLogger("juju.audit")

// Audit records an auditable event against the user who performed the action
func Audit(user *state.User, format string, args ...interface{}) {
	if user == nil {
		panic("user cannot be nil")
	}
	// TODO(dfc) we should also refuse to accept state.User objects that
	// do not have a name, ie they are blank.
	logger.Logf(loggo.INFO, fmt.Sprintf("user %q: %s", user.Name(), format), args...)
}
