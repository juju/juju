// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools_test

import (
	stdtesting "testing"

	"github.com/juju/juju/v2/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
