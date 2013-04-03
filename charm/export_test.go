package charm

import (
	"launchpad.net/juju-core/schema"
)

// Export meaningful bits for tests only.

func IfaceExpander(limit interface{}) schema.Checker {
	return ifaceExpander(limit)
}

func NewStore(url string) *CharmStore {
	return &CharmStore{url}
}
