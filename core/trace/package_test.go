// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

func TestAll(t *stdtesting.T) {
	tc.TestingT(t)
}
