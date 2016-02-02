// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent // not agent_test for no good reason

import (
	stdtesting "testing"

	"github.com/juju/testing"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	if testing.RaceEnabled {
		t.Skip("skipping package under -race, see LP 1519133, 1519097")
	}
	// TODO(waigani) 2014-03-19 bug 1294458
	// Refactor to use base suites
	coretesting.MgoTestPackage(t)
}
