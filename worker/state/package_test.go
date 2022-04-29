// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

var NewStateTracker = newStateTracker

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
