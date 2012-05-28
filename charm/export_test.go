package charm

import (
	"launchpad.net/juju/go/schema"
)

// Export meaningful bits for tests only.

func IfaceExpander(limit interface{}) schema.Checker {
	return ifaceExpander(limit)
}

func NewStore(url, path string) Repository {
	return &store{url, path}
}
