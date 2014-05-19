// Package audit records auditable events
package audit

import (
	"fmt"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/state"
)

var auditLogger = loggo.GetLogger("audit")

// Audit records an auditable event against the user who performed the action
func Audit(user *state.User, format string, args ...interface{}) {
	if user == nil {
		panic("user cannot be nil")
	}
	auditLogger.Infof(fmt.Sprintf("user %q: %s", user, format), args...)
}
