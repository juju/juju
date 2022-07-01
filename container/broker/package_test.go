// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker_test

import (
	stdtesting "testing"

	"github.com/juju/juju/v3/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
