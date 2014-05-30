// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	stdtesting "testing"

	"launchpad.net/juju-core/testing"
)

func Test(t *stdtesting.T) {
	testing.MgoTestPackageSsl(t, false)
}
