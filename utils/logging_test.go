// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/utils"
)

type logSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&logSuite{})

func (*logSuite) TestFlagNotSet(c *gc.C) {
	err := errors.New("test error")
	err2 := utils.LoggedErrorStack(err)
	c.Assert(err, gc.Equals, err2)
	c.Assert(c.GetTestLog(), gc.Equals, "")
}

func (s *logSuite) TestFlagSet(c *gc.C) {
	s.SetFeatureFlags(feature.LogErrorStack)
	err := errors.New("test error")
	err2 := utils.LoggedErrorStack(err)
	c.Assert(err, gc.Equals, err2)
	expected := "ERROR juju.utils error stack:\ntest error"
	c.Assert(c.GetTestLog(), jc.Contains, expected)
}
