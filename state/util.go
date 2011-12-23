// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

import (
	"errors"
)

var (
	ErrIncompatibleVersion = errors.New("state: loaded topology has incompatible version")
	ErrServiceNotFound     = errors.New("state: named service cannot be found")
	ErrServiceHasNoCharmId = errors.New("state: service has no charm id")
	ErrUnitNotFound        = errors.New("state: named unit cannot be found")
)
