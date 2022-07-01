// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcache_test

import (
	stdtesting "testing"

	"github.com/juju/juju/v2/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
