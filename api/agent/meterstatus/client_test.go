// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/meterstatus"
	"github.com/juju/juju/api/base/testing"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type meterStatusSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&meterStatusSuite{})

func (s *meterStatusSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *meterStatusSuite) TestGetMeterStatus(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, gc.Equals, "MeterStatus")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetMeterStatus")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.MeterStatusResults{})
		result := response.(*params.MeterStatusResults)
		result.Results = []params.MeterStatusResult{{
			Code: "GREEN",
			Info: "All ok.",
		}}
		called = true
		return nil
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, gc.NotNil)

	statusCode, statusInfo, err := status.MeterStatus()
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusCode, gc.Equals, "GREEN")
	c.Assert(statusInfo, gc.Equals, "All ok.")
}

func (s *meterStatusSuite) TestGetMeterStatusNotImplemented(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})

	tag := names.NewUnitTag("wp/1")
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, gc.NotNil)

	_, _, err := status.MeterStatus()
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *meterStatusSuite) TestGetMeterStatusResultError(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, gc.Equals, "MeterStatus")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetMeterStatus")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.MeterStatusResults{})
		result := response.(*params.MeterStatusResults)
		result.Results = []params.MeterStatusResult{{
			Error: &params.Error{
				Message: "An error in the meter status.",
				Code:    params.CodeNotAssigned,
			},
		}}
		called = true
		return nil
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, gc.NotNil)

	statusCode, statusInfo, err := status.MeterStatus()
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "An error in the meter status.")
	c.Assert(statusCode, gc.Equals, "")
	c.Assert(statusInfo, gc.Equals, "")
}

func (s *meterStatusSuite) TestGetMeterStatusReturnsError(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, gc.Equals, "MeterStatus")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetMeterStatus")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.MeterStatusResults{})
		called = true
		return fmt.Errorf("could not retrieve meter status")
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, gc.NotNil)

	statusCode, statusInfo, err := status.MeterStatus()
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "could not retrieve meter status")
	c.Assert(statusCode, gc.Equals, "")
	c.Assert(statusInfo, gc.Equals, "")
}

func (s *meterStatusSuite) TestGetMeterStatusMoreResults(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, gc.Equals, "MeterStatus")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetMeterStatus")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.MeterStatusResults{})
		result := response.(*params.MeterStatusResults)
		result.Results = make([]params.MeterStatusResult, 2)
		called = true
		return nil
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, gc.NotNil)
	statusCode, statusInfo, err := status.MeterStatus()
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(statusCode, gc.Equals, "")
	c.Assert(statusInfo, gc.Equals, "")
}

func (s *meterStatusSuite) TestWatchMeterStatusError(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, gc.Equals, "MeterStatus")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchMeterStatus")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = make([]params.NotifyWatchResult, 1)
		called = true
		return fmt.Errorf("could not retrieve meter status watcher")
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, gc.NotNil)
	w, err := status.WatchMeterStatus()
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "could not retrieve meter status watcher")
	c.Assert(w, gc.IsNil)
}

func (s *meterStatusSuite) TestWatchMeterStatusNotImplemented(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})

	tag := names.NewUnitTag("wp/1")
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, gc.NotNil)

	_, err := status.WatchMeterStatus()
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *meterStatusSuite) TestWatchMeterStatusMoreResults(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, gc.Equals, "MeterStatus")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchMeterStatus")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = make([]params.NotifyWatchResult, 2)
		called = true
		return nil
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, gc.NotNil)
	w, err := status.WatchMeterStatus()
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(w, gc.IsNil)
}

func (s *meterStatusSuite) TestWatchMeterStatusResultError(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, gc.Equals, "MeterStatus")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchMeterStatus")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = []params.NotifyWatchResult{{
			Error: &params.Error{
				Message: "error",
				Code:    params.CodeNotAssigned,
			},
		}}

		called = true
		return nil
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, gc.NotNil)
	w, err := status.WatchMeterStatus()
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "error")
	c.Assert(w, gc.IsNil)
}
