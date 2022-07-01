// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	stdtesting "testing"

	"github.com/juju/juju/v2/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
