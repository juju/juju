// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"testing"

	"github.com/juju/tc"
)

type addUnitSuite struct{}

func TestAddUnit(t *testing.T) {
	tc.Run(t, &addUnitSuite{})
}
