// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	// TODO(fwereade): there's no good reason for this test to use mongo.
	coretesting.MgoTestPackage(t)
}
