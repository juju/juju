// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/juju/state"
)

// The interfaces below are used to create mocks for testing.

type Backend interface {
	Model() (*state.Model, error)
}
