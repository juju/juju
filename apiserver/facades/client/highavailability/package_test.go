// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	stdtesting "testing"

	"github.com/juju/juju/internal/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
