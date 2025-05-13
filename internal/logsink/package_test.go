// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/goleak"
)

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}
