// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	stdtesting "testing"

	"github.com/juju/testing"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	if testing.RaceEnabled {
		t.Skip("skipping package under -race, see LP 1519097")
	}
	coretesting.MgoTestPackage(t)
}
