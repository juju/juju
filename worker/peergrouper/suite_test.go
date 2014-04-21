// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper_test

import (
	stdtesting "testing"

	"launchpad.net/juju-core/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
