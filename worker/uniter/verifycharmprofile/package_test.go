// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package verifycharmprofile_test

import (
	stdtesting "testing"

	"github.com/juju/juju/v2/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
