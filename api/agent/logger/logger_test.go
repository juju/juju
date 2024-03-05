// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/logger"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type loggerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&loggerSuite{})

func (s *loggerSuite) TestLoggingConfig(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Logger")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "LoggingConfig")
		c.Check(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: "machine-666",
		}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{Result: "juju.worker=TRACE"}},
		}
		return nil
	})

	client := logger.NewClient(apiCaller)
	tag := names.NewMachineTag("666")
	result, err := client.LoggingConfig(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "juju.worker=TRACE")
}

func (s *loggerSuite) TestWatchLoggingConfig(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Logger")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchLoggingConfig")
		c.Check(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: "machine-666",
		}}})
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := logger.NewClient(apiCaller)
	tag := names.NewMachineTag("666")
	_, err := client.WatchLoggingConfig(tag)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}
