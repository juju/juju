package charm

import (
	"launchpad.net/juju/go/schema"
)

// Export meaningful bits for tests only.

func IfaceExpander(limit interface{}) schema.Checker {
	return ifaceExpander(limit)
}
