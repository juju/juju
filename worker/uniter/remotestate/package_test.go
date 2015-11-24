// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate_test

import (
	"testing"

	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	if jujutesting.RaceEnabled {
		t.Skip("skipping package under -race, see LP 1519149")
	}
	gc.TestingT(t)
}
