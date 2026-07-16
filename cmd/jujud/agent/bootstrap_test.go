// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

type BootstrapSuite struct {
}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &BootstrapSuite{})
}
