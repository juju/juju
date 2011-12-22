// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

import (
	"errors"
)

var (
	ErrServiceNotFound = errors.New("state: named service cannot be found")
	ErrUnitNotFound    = errors.New("service: named unit cannot be found")
)
