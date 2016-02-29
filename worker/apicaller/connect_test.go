// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type ScaryConnectSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ScaryConnectSuite{})

func (s *ScaryConnectSuite) TestFatal(c *gc.C) {
	c.Fatalf("xxx")
}
