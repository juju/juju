// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/machineactions"
	"github.com/juju/juju/apiserver/params"
)

type ClientSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) TestWatchFails(c *gc.C) {
	tag := names.NewMachineTag("2")
	expectErr := errors.Errorf("kuso")
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.WatchActionNotifications",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		res := result.(*params.StringsWatchResults)
		res.Results = make([]params.StringsWatchResult, 1)
		return expectErr
	})

	client := machineactions.NewClient(apiCaller)
	w, err := client.WatchActionNotifications(tag)
	c.Assert(errors.Cause(err), gc.Equals, expectErr)
	c.Assert(w, gc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestWatchResultError(c *gc.C) {
	tag := names.NewMachineTag("2")
	expectErr := &params.Error{
		Message: "rigged",
		Code:    params.CodeNotAssigned,
	}
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.WatchActionNotifications",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		res := result.(*params.StringsWatchResults)
		res.Results = make([]params.StringsWatchResult, 1)
		res.Results[0].Error = expectErr
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	w, err := client.WatchActionNotifications(tag)
	c.Assert(errors.Cause(err), gc.Equals, expectErr)
	c.Assert(w, gc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestWatchResultTooMany(c *gc.C) {
	tag := names.NewMachineTag("2")
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.WatchActionNotifications",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		res := result.(*params.StringsWatchResults)
		res.Results = make([]params.StringsWatchResult, 2)
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	w, err := client.WatchActionNotifications(tag)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(w, gc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionBeginSuccess(c *gc.C) {
	tag := names.NewActionTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.BeginActions",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}

		return nil
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionBegin(tag)
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionBeginError(c *gc.C) {
	tag := names.NewActionTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.BeginActions",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	expectedErr := errors.Errorf("blam")
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ErrorResults{})
		return expectedErr
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionBegin(tag)
	c.Assert(errors.Cause(err), gc.Equals, expectedErr)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionBeginResultError(c *gc.C) {
	tag := names.NewActionTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.BeginActions",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	expectedErr := &params.Error{
		Message: "rigged",
		Code:    params.CodeNotAssigned,
	}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{expectedErr}},
		}

		return nil
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionBegin(tag)
	c.Assert(errors.Cause(err), gc.Equals, expectedErr)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionBeginTooManyResults(c *gc.C) {
	tag := names.NewActionTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.BeginActions",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ErrorResults{})
		res := result.(*params.ErrorResults)
		res.Results = make([]params.ErrorResult, 2)
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionBegin(tag)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionFinishSuccess(c *gc.C) {
	tag := names.NewActionTag(utils.MustNewUUID().String())
	status := "stubstatus"
	actionResults := map[string]interface{}{"stub": "stub"}
	message := "stubmsg"
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.FinishActions",
		[]interface{}{"", params.ActionExecutionResults{
			Results: []params.ActionExecutionResult{{
				ActionTag: tag.String(),
				Status:    status,
				Results:   actionResults,
				Message:   message,
			}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionFinish(tag, status, actionResults, message)
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionFinishError(c *gc.C) {
	tag := names.NewActionTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.FinishActions",
		[]interface{}{"", params.ActionExecutionResults{
			Results: []params.ActionExecutionResult{{
				ActionTag: tag.String(),
				Status:    "",
				Results:   nil,
				Message:   "",
			}},
		}},
	}}
	expectedErr := errors.Errorf("blam")
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ErrorResults{})
		return expectedErr
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionFinish(tag, "", nil, "")
	c.Assert(errors.Cause(err), gc.Equals, expectedErr)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionFinishResultError(c *gc.C) {
	tag := names.NewActionTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.FinishActions",
		[]interface{}{"", params.ActionExecutionResults{
			Results: []params.ActionExecutionResult{{
				ActionTag: tag.String(),
				Status:    "",
				Results:   nil,
				Message:   "",
			}},
		}},
	}}
	expectedErr := &params.Error{
		Message: "rigged",
		Code:    params.CodeNotAssigned,
	}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{expectedErr}},
		}

		return nil
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionFinish(tag, "", nil, "")
	c.Assert(errors.Cause(err), gc.Equals, expectedErr)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionFinishTooManyResults(c *gc.C) {
	tag := names.NewActionTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.FinishActions",
		[]interface{}{"", params.ActionExecutionResults{
			Results: []params.ActionExecutionResult{{
				ActionTag: tag.String(),
				Status:    "",
				Results:   nil,
				Message:   "",
			}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ErrorResults{})
		res := result.(*params.ErrorResults)
		res.Results = make([]params.ErrorResult, 2)
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionFinish(tag, "", nil, "")
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestGetActionSuccess(c *gc.C) {
	tag := names.NewActionTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.Actions",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	expectedName := "ack"
	expectedParams := map[string]interface{}{"floob": "zgloob"}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ActionResults{})
		*(result.(*params.ActionResults)) = params.ActionResults{
			Results: []params.ActionResult{{
				Action: &params.Action{
					Name:       expectedName,
					Parameters: expectedParams,
				},
			}},
		}
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	action, err := client.Action(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(action.Name(), gc.Equals, expectedName)
	c.Assert(action.Params(), gc.DeepEquals, expectedParams)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestGetActionError(c *gc.C) {
	tag := names.NewActionTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.Actions",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	expectedErr := errors.Errorf("blam")
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ActionResults{})
		return expectedErr
	})

	client := machineactions.NewClient(apiCaller)
	action, err := client.Action(tag)
	c.Assert(errors.Cause(err), gc.Equals, expectedErr)
	c.Assert(action, gc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestGetActionResultError(c *gc.C) {
	tag := names.NewActionTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.Actions",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	expectedErr := &params.Error{
		Message: "rigged",
		Code:    params.CodeNotAssigned,
	}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ActionResults{})
		*(result.(*params.ActionResults)) = params.ActionResults{
			Results: []params.ActionResult{{
				Error: expectedErr,
			}},
		}
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	action, err := client.Action(tag)
	c.Assert(errors.Cause(err), gc.Equals, expectedErr)
	c.Assert(action, gc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestGetActionTooManyResults(c *gc.C) {
	tag := names.NewActionTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.Actions",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ActionResults{})
		res := result.(*params.ActionResults)
		res.Results = make([]params.ActionResult, 2)
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	action, err := client.Action(tag)
	c.Assert(err, gc.ErrorMatches, "expected only 1 action query result, got 2")
	c.Assert(action, gc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestRunningActionSuccess(c *gc.C) {
	tag := names.NewMachineTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.RunningActions",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	actionsList := []params.ActionResult{
		{Action: &params.Action{Name: "foo"}},
		{Action: &params.Action{Name: "baz"}},
	}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ActionsByReceivers{})
		*(result.(*params.ActionsByReceivers)) = params.ActionsByReceivers{
			Actions: []params.ActionsByReceiver{{
				Actions: actionsList,
			}},
		}
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	actions, err := client.RunningActions(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, jc.DeepEquals, actionsList)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestRunningActionsError(c *gc.C) {
	tag := names.NewMachineTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.RunningActions",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	expectedErr := errors.Errorf("blam")
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ActionsByReceivers{})
		return expectedErr
	})

	client := machineactions.NewClient(apiCaller)
	actions, err := client.RunningActions(tag)
	c.Assert(errors.Cause(err), gc.Equals, expectedErr)
	c.Assert(actions, gc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestRunningActionsResultError(c *gc.C) {
	tag := names.NewMachineTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.RunningActions",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	expectedErr := &params.Error{
		Message: "rigged",
		Code:    params.CodeNotAssigned,
	}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ActionsByReceivers{})
		*(result.(*params.ActionsByReceivers)) = params.ActionsByReceivers{
			Actions: []params.ActionsByReceiver{{
				Error: expectedErr,
			}},
		}
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	action, err := client.RunningActions(tag)
	c.Assert(errors.Cause(err), gc.Equals, expectedErr)
	c.Assert(action, gc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestRunningActionsTooManyResults(c *gc.C) {
	tag := names.NewMachineTag(utils.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		"MachineActions.RunningActions",
		[]interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, gc.FitsTypeOf, &params.ActionsByReceivers{})
		res := result.(*params.ActionsByReceivers)
		res.Actions = make([]params.ActionsByReceiver, 2)
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	actions, err := client.RunningActions(tag)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(actions, gc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}
