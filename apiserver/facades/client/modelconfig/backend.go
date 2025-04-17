// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"github.com/juju/names/v6"
)

// Backend contains the state.State methods used in this package,
// allowing stubs to be created for testing.
type Backend interface {
	ControllerTag() names.ControllerTag
	Sequences() (map[string]int, error)
}
