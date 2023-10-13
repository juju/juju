// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"errors"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/testing"
)

type logSuite struct {
	testing.LoggingSuite
	jujutesting.JujuOSEnvSuite
	logger loggo.Logger
}

func (l *logSuite) SetUpTest(c *gc.C) {
	l.LoggingSuite.SetUpTest(c)
	l.JujuOSEnvSuite.SetUpTest(c)
	l.logger = loggo.GetLogger("juju.provisioner")
}

var _ = gc.Suite(&logSuite{})

func (s *logSuite) TestFlagNotSet(c *gc.C) {
	err := errors.New("test error")
	err2 := loggedErrorStack(s.logger, err)
	c.Assert(err, gc.Equals, err2)
	//c.Assert(c.GetTestLog(), gc.Equals, "")
}

func (s *logSuite) TestFlagSet(c *gc.C) {
	s.SetFeatureFlags(feature.LogErrorStack)
	err := errors.New("test error")
	err2 := loggedErrorStack(s.logger, err)
	c.Assert(err, gc.Equals, err2)
	//expected := "ERROR juju.provisioner error stack:\ntest error"
	//c.Assert(c.GetTestLog(), jc.Contains, expected)
}
