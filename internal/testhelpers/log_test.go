// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers

import (
	"testing"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
)

type logSuite struct{}

func TestLogSuite(t *testing.T) {
	tc.Run(t, &logSuite{})
}

func (*logSuite) TestLog(c *tc.C) {
	logger := loggo.GetLogger("test")
	jujuLogger := loggo.GetLogger("juju")
	logConfig = "<root>=DEBUG;juju=TRACE"

	c.Assert(logger.EffectiveLogLevel(), tc.Equals, loggo.WARNING)
	var suite LoggingSuite
	suite.SetUpSuite(c)

	c.Assert(logger.EffectiveLogLevel(), tc.Equals, loggo.DEBUG)
	c.Assert(jujuLogger.EffectiveLogLevel(), tc.Equals, loggo.TRACE)

	logger.Debugf("message 1")
	logger.Tracef("message 2")
	jujuLogger.Tracef("message 3")

	//c.Assert(c.GetTestLog(), tc.Matches,
	//	".*DEBUG test message 1\n"+
	//		".*TRACE juju message 3\n",
	//)
	suite.TearDownSuite(c)
	logger.Debugf("message 1")
	logger.Tracef("message 2")
	jujuLogger.Tracef("message 3")

	//c.Assert(c.GetTestLog(), tc.Matches,
	//	".*DEBUG test message 1\n"+
	//		".*TRACE juju message 3\n$",
	//)
	c.Assert(logger.EffectiveLogLevel(), tc.Equals, loggo.WARNING)
	c.Assert(jujuLogger.EffectiveLogLevel(), tc.Equals, loggo.WARNING)
}
