// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/v2/api/base/testing"
	"github.com/juju/juju/v2/api/client/action"
	"github.com/juju/juju/v2/rpc/params"
)

type runSuite struct{}

var _ = gc.Suite(&runSuite{})

func (s *actionSuite) TestRunOnAllMachinesLegacy(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Assert(request, gc.Equals, "RunOnAllMachines")
				c.Assert(a, jc.DeepEquals, params.RunParams{
					Commands: "pwd", Timeout: time.Millisecond})
				c.Assert(result, gc.FitsTypeOf, &params.ActionResults{})
				*(result.(*params.ActionResults)) = params.ActionResults{
					Results: []params.ActionResult{{
						Action: &params.Action{
							Tag:      "action-1",
							Receiver: "machine-0",
						},
					}},
				}
				return nil
			},
		),
		BestVersion: 6,
	}
	client := action.NewClient(apiCaller)
	result, err := client.RunOnAllMachines("pwd", time.Millisecond)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, action.EnqueuedActions{
		Actions: []action.ActionResult{{
			Action: &action.Action{
				ID:       "1",
				Receiver: "machine-0",
			},
		}},
	})
}

func (s *actionSuite) TestRunOnAllMachines(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Assert(request, gc.Equals, "RunOnAllMachines")
				c.Assert(a, jc.DeepEquals, params.RunParams{
					Commands: "pwd", Timeout: time.Millisecond})
				c.Assert(result, gc.FitsTypeOf, &params.EnqueuedActionsV2{})
				*(result.(*params.EnqueuedActionsV2)) = params.EnqueuedActionsV2{
					OperationTag: "operation-666",
					Actions: []params.ActionResult{{
						Action: &params.Action{
							Tag:      "action-1",
							Receiver: "machine-0",
						}}},
				}
				return nil
			},
		),
		BestVersion: 7,
	}
	client := action.NewClient(apiCaller)
	result, err := client.RunOnAllMachines("pwd", time.Millisecond)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, action.EnqueuedActions{
		OperationID: "666",
		Actions: []action.ActionResult{{
			Action: &action.Action{
				ID:       "1",
				Receiver: "machine-0",
			},
		}},
	})
}

func (s *actionSuite) TestRunLegacy(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Assert(request, gc.Equals, "Run")
				c.Assert(a, jc.DeepEquals, params.RunParams{
					Commands: "pwd",
					Timeout:  time.Millisecond,
					Machines: []string{"0"},
				})
				c.Assert(result, gc.FitsTypeOf, &params.ActionResults{})
				*(result.(*params.ActionResults)) = params.ActionResults{
					Results: []params.ActionResult{{
						Action: &params.Action{
							Tag:      "action-1",
							Receiver: "machine-0",
						},
					}},
				}
				return nil
			},
		),
		BestVersion: 6,
	}
	client := action.NewClient(apiCaller)
	result, err := client.Run(action.RunParams{
		Commands: "pwd",
		Timeout:  time.Millisecond,
		Machines: []string{"0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, action.EnqueuedActions{
		Actions: []action.ActionResult{{
			Action: &action.Action{
				ID:       "1",
				Receiver: "machine-0",
			},
		}},
	})
}

func (s *actionSuite) TestRun(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Assert(request, gc.Equals, "Run")
				c.Assert(a, jc.DeepEquals, params.RunParams{
					Commands: "pwd",
					Timeout:  time.Millisecond,
					Machines: []string{"0"},
				})
				c.Assert(result, gc.FitsTypeOf, &params.EnqueuedActionsV2{})
				*(result.(*params.EnqueuedActionsV2)) = params.EnqueuedActionsV2{
					OperationTag: "operation-666",
					Actions: []params.ActionResult{{
						Action: &params.Action{
							Tag:      "action-1",
							Receiver: "machine-0",
						}}},
				}
				return nil
			},
		),
		BestVersion: 7,
	}
	client := action.NewClient(apiCaller)
	result, err := client.Run(action.RunParams{
		Commands: "pwd",
		Timeout:  time.Millisecond,
		Machines: []string{"0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, action.EnqueuedActions{
		OperationID: "666",
		Actions: []action.ActionResult{{
			Action: &action.Action{
				ID:       "1",
				Receiver: "machine-0",
			},
		}},
	})
}
