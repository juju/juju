// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"github.com/juju/juju/state"
)

type Backend interface{}

type stateShim struct {
	*state.State
}
