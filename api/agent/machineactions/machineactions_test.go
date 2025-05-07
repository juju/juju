// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api/agent/machineactions"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type ClientSuite struct {
	jujutesting.IsolationSuite
}

var _ = tc.Suite(&ClientSuite{})

func (s *ClientSuite) TestWatchFails(c *tc.C) {
	tag := names.NewMachineTag("2")
	expectErr := errors.Errorf("kuso")
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.WatchActionNotifications",
		Args: []interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		res := result.(*params.StringsWatchResults)
		res.Results = make([]params.StringsWatchResult, 1)
		return expectErr
	})

	client := machineactions.NewClient(apiCaller)
	w, err := client.WatchActionNotifications(context.Background(), tag)
	c.Assert(errors.Cause(err), tc.Equals, expectErr)
	c.Assert(w, tc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestWatchResultError(c *tc.C) {
	tag := names.NewMachineTag("2")
	expectErr := &params.Error{
		Message: "rigged",
		Code:    params.CodeNotAssigned,
	}
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.WatchActionNotifications",
		Args: []interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		res := result.(*params.StringsWatchResults)
		res.Results = make([]params.StringsWatchResult, 1)
		res.Results[0].Error = expectErr
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	w, err := client.WatchActionNotifications(context.Background(), tag)
	c.Assert(errors.Cause(err), tc.Equals, expectErr)
	c.Assert(w, tc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestWatchResultTooMany(c *tc.C) {
	tag := names.NewMachineTag("2")
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.WatchActionNotifications",
		Args: []interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		res := result.(*params.StringsWatchResults)
		res.Results = make([]params.StringsWatchResult, 2)
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	w, err := client.WatchActionNotifications(context.Background(), tag)
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(w, tc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionBeginSuccess(c *tc.C) {
	tag := names.NewActionTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.BeginActions",
		Args: []interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}

		return nil
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionBegin(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionBeginError(c *tc.C) {
	tag := names.NewActionTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.BeginActions",
		Args: []interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	expectedErr := errors.Errorf("blam")
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, tc.FitsTypeOf, &params.ErrorResults{})
		return expectedErr
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionBegin(context.Background(), tag)
	c.Assert(errors.Cause(err), tc.Equals, expectedErr)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionBeginResultError(c *tc.C) {
	tag := names.NewActionTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.BeginActions",
		Args: []interface{}{"", params.Entities{
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
		c.Check(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{expectedErr}},
		}

		return nil
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionBegin(context.Background(), tag)
	c.Assert(errors.Cause(err), tc.Equals, expectedErr)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionBeginTooManyResults(c *tc.C) {
	tag := names.NewActionTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.BeginActions",
		Args: []interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, tc.FitsTypeOf, &params.ErrorResults{})
		res := result.(*params.ErrorResults)
		res.Results = make([]params.ErrorResult, 2)
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionBegin(context.Background(), tag)
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionFinishSuccess(c *tc.C) {
	tag := names.NewActionTag(uuid.MustNewUUID().String())
	status := "stubstatus"
	actionResults := map[string]interface{}{"stub": "stub"}
	message := "stubmsg"
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.FinishActions",
		Args: []interface{}{"", params.ActionExecutionResults{
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
		c.Check(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionFinish(context.Background(), tag, status, actionResults, message)
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionFinishError(c *tc.C) {
	tag := names.NewActionTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.FinishActions",
		Args: []interface{}{"", params.ActionExecutionResults{
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
		c.Check(result, tc.FitsTypeOf, &params.ErrorResults{})
		return expectedErr
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionFinish(context.Background(), tag, "", nil, "")
	c.Assert(errors.Cause(err), tc.Equals, expectedErr)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionFinishResultError(c *tc.C) {
	tag := names.NewActionTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.FinishActions",
		Args: []interface{}{"", params.ActionExecutionResults{
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
		c.Check(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: expectedErr}},
		}

		return nil
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionFinish(context.Background(), tag, "", nil, "")
	c.Assert(errors.Cause(err), tc.Equals, expectedErr)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestActionFinishTooManyResults(c *tc.C) {
	tag := names.NewActionTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.FinishActions",
		Args: []interface{}{"", params.ActionExecutionResults{
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
		c.Check(result, tc.FitsTypeOf, &params.ErrorResults{})
		res := result.(*params.ErrorResults)
		res.Results = make([]params.ErrorResult, 2)
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	err := client.ActionFinish(context.Background(), tag, "", nil, "")
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestGetActionSuccess(c *tc.C) {
	tag := names.NewActionTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.Actions",
		Args: []interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	expectedName := "ack"
	expectedParams := map[string]interface{}{"floob": "zgloob"}
	var stub jujutesting.Stub

	parallel := true
	group := "group"
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, tc.FitsTypeOf, &params.ActionResults{})
		*(result.(*params.ActionResults)) = params.ActionResults{
			Results: []params.ActionResult{{
				Action: &params.Action{
					Name:           expectedName,
					Parameters:     expectedParams,
					Parallel:       &parallel,
					ExecutionGroup: &group,
				},
			}},
		}
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	action, err := client.Action(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(action.Name(), tc.Equals, expectedName)
	c.Assert(action.Params(), tc.DeepEquals, expectedParams)
	c.Assert(action.Parallel(), jc.IsTrue)
	c.Assert(action.ExecutionGroup(), tc.Equals, "group")
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestGetActionError(c *tc.C) {
	tag := names.NewActionTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.Actions",
		Args: []interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	expectedErr := errors.Errorf("blam")
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, tc.FitsTypeOf, &params.ActionResults{})
		return expectedErr
	})

	client := machineactions.NewClient(apiCaller)
	action, err := client.Action(context.Background(), tag)
	c.Assert(errors.Cause(err), tc.Equals, expectedErr)
	c.Assert(action, tc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestGetActionResultError(c *tc.C) {
	tag := names.NewActionTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.Actions",
		Args: []interface{}{"", params.Entities{
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
		c.Check(result, tc.FitsTypeOf, &params.ActionResults{})
		*(result.(*params.ActionResults)) = params.ActionResults{
			Results: []params.ActionResult{{
				Error: expectedErr,
			}},
		}
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	action, err := client.Action(context.Background(), tag)
	c.Assert(errors.Cause(err), tc.Equals, expectedErr)
	c.Assert(action, tc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestGetActionTooManyResults(c *tc.C) {
	tag := names.NewActionTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.Actions",
		Args: []interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, tc.FitsTypeOf, &params.ActionResults{})
		res := result.(*params.ActionResults)
		res.Results = make([]params.ActionResult, 2)
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	action, err := client.Action(context.Background(), tag)
	c.Assert(err, tc.ErrorMatches, "expected only 1 action query result, got 2")
	c.Assert(action, tc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestRunningActionSuccess(c *tc.C) {
	tag := names.NewMachineTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.RunningActions",
		Args: []interface{}{"", params.Entities{
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
		c.Check(result, tc.FitsTypeOf, &params.ActionsByReceivers{})
		*(result.(*params.ActionsByReceivers)) = params.ActionsByReceivers{
			Actions: []params.ActionsByReceiver{{
				Actions: actionsList,
			}},
		}
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	actions, err := client.RunningActions(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, jc.DeepEquals, actionsList)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestRunningActionsError(c *tc.C) {
	tag := names.NewMachineTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.RunningActions",
		Args: []interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	expectedErr := errors.Errorf("blam")
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, tc.FitsTypeOf, &params.ActionsByReceivers{})
		return expectedErr
	})

	client := machineactions.NewClient(apiCaller)
	actions, err := client.RunningActions(context.Background(), tag)
	c.Assert(errors.Cause(err), tc.Equals, expectedErr)
	c.Assert(actions, tc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestRunningActionsResultError(c *tc.C) {
	tag := names.NewMachineTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.RunningActions",
		Args: []interface{}{"", params.Entities{
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
		c.Check(result, tc.FitsTypeOf, &params.ActionsByReceivers{})
		*(result.(*params.ActionsByReceivers)) = params.ActionsByReceivers{
			Actions: []params.ActionsByReceiver{{
				Error: expectedErr,
			}},
		}
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	action, err := client.RunningActions(context.Background(), tag)
	c.Assert(errors.Cause(err), tc.Equals, expectedErr)
	c.Assert(action, tc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}

func (s *ClientSuite) TestRunningActionsTooManyResults(c *tc.C) {
	tag := names.NewMachineTag(uuid.MustNewUUID().String())
	expectedCalls := []jujutesting.StubCall{{
		FuncName: "MachineActions.RunningActions",
		Args: []interface{}{"", params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		}},
	}}
	var stub jujutesting.Stub

	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		c.Check(result, tc.FitsTypeOf, &params.ActionsByReceivers{})
		res := result.(*params.ActionsByReceivers)
		res.Actions = make([]params.ActionsByReceiver, 2)
		return nil
	})

	client := machineactions.NewClient(apiCaller)
	actions, err := client.RunningActions(context.Background(), tag)
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(actions, tc.IsNil)
	stub.CheckCalls(c, expectedCalls)
}
