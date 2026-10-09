// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

type recoverySuite struct{}

func TestRecoverySuite(t *stdtesting.T) {
	tc.Run(t, &recoverySuite{})
}
