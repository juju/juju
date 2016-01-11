// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
