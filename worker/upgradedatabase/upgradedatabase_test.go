// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase_test

import (
	"github.com/juju/loggo"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/upgradedatabase"
)

// baseSuite is embedded in both the worker and manifold tests.
// Tests should not go on this suite directly.
type baseSuite struct {
	testing.IsolationSuite

	logger *upgradedatabase.MockLogger
}

// ignoreLogging turns the suite's mock logger into a sink, with no validation.
// Logs are still emitted via the test logger.
func (s *baseSuite) ignoreLogging(c *gc.C) {
	debugIt := func(message string, args ...any) { logIt(c, loggo.DEBUG, message, args) }
	infoIt := func(message string, args ...any) { logIt(c, loggo.INFO, message, args) }
	warningIt := func(message string, args ...any) { logIt(c, loggo.WARNING, message, args) }
	errorIt := func(message string, args ...any) { logIt(c, loggo.ERROR, message, args) }

	e := s.logger.EXPECT()
	e.Debugf(gomock.Any(), gomock.Any()).AnyTimes().Do(debugIt)
	e.Infof(gomock.Any(), gomock.Any()).AnyTimes().Do(infoIt)
	e.Warningf(gomock.Any(), gomock.Any()).AnyTimes().Do(warningIt)
	e.Errorf(gomock.Any(), gomock.Any()).AnyTimes().Do(errorIt)
}

func logIt(c *gc.C, level loggo.Level, message string, args interface{}) {
	var nArgs []interface{}
	var ok bool
	if nArgs, ok = args.([]interface{}); ok {
		nArgs = append([]interface{}{level}, nArgs...)
	} else {
		nArgs = append([]interface{}{level}, args)
	}

	c.Logf("%s "+message, nArgs...)
}
