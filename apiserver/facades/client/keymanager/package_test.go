// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

import (
	stdtesting "testing"

	"github.com/juju/juju/v3/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
