// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"testing"

	"github.com/juju/tc"
)

func Test(t *testing.T) {
	setupToolsTests()
	setupSimpleStreamsTests(t)
	tc.TestingT(t)
}
