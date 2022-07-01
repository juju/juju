// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/v2/testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
