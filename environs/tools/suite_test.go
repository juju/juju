// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"os"
	"testing"

	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	setupToolsTests()
	setupSimpleStreamsTests(t)
	gc.TestingT(t)
}

func TestMain(m *testing.M) {
	jujutesting.ExecHelperProcess()
	os.Exit(m.Run())
}
