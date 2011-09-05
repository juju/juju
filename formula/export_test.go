package formula

import (
	"launchpad.net/ensemble/go/schema"
)

// Export meaningful bits for tests only.

func IfaceExpander(limit interface{}) schema.Checker {
	return ifaceExpander(limit)
}
