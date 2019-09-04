// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	if !runStateTests {
		t.Skip("skipping state tests since the skip_state_tests build tag was set")
	}
	coretesting.MgoTestPackage(t)
}
