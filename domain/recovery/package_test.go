// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type recoverySuite struct {
	testhelpers.IsolationSuite
}

func TestRecoverySuite(t *stdtesting.T) {
	tc.Run(t, &recoverySuite{})
}
