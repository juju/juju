// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"testing"

	gc "launchpad.net/gocheck"
)

func Test(t *testing.T) {
	setupToolsTests()
	setupSimpleStreamsTests(t)
	gc.TestingT(t)
}
