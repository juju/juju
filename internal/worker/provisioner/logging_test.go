// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"errors"
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/feature"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujutesting "github.com/juju/juju/testing"
)

type logSuite struct {
	testing.LoggingSuite
	jujutesting.JujuOSEnvSuite
}

func (l *logSuite) SetUpTest(c *gc.C) {
	l.LoggingSuite.SetUpTest(c)
	l.JujuOSEnvSuite.SetUpTest(c)
}

var _ = gc.Suite(&logSuite{})

func (s *logSuite) TestFlagNotSet(c *gc.C) {
	var entries []string
	recorder := loggertesting.RecordLog(func(s string, a ...any) {
		entries = append(entries, s)
	})
	logger := loggertesting.WrapCheckLog(recorder)

	err := errors.New("test error")
	err2 := loggedErrorStack(logger, err)
	c.Assert(err, gc.Equals, err2)
	c.Assert(entries, gc.HasLen, 0)
}

func (s *logSuite) TestFlagSet(c *gc.C) {
	var entries []string
	recorder := loggertesting.RecordLog(func(s string, a ...any) {
		entries = append(entries, fmt.Sprintf(s, a...))
	})
	logger := loggertesting.WrapCheckLog(recorder)

	s.SetFeatureFlags(feature.LogErrorStack)
	err := errors.New("test error")
	err2 := loggedErrorStack(logger, err)
	c.Assert(err, gc.Equals, err2)
	c.Assert(entries, jc.SameContents, []string{
		"ERROR: error stack:\n[test error]",
	})
}
