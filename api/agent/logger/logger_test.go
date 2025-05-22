// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/logger"
	"github.com/juju/juju/api/base/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type loggerSuite struct {
	coretesting.BaseSuite
}

func TestLoggerSuite(t *stdtesting.T) {
	tc.Run(t, &loggerSuite{})
}

func (s *loggerSuite) TestLoggingConfig(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Logger")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "LoggingConfig")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: "machine-666",
		}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{Result: "juju.worker=TRACE"}},
		}
		return nil
	})

	client := logger.NewClient(apiCaller)
	tag := names.NewMachineTag("666")
	result, err := client.LoggingConfig(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, "juju.worker=TRACE")
}

func (s *loggerSuite) TestWatchLoggingConfig(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Logger")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchLoggingConfig")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: "machine-666",
		}}})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := logger.NewClient(apiCaller)
	tag := names.NewMachineTag("666")
	_, err := client.WatchLoggingConfig(c.Context(), tag)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}
