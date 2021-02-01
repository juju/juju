// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/action"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/params"
)

type runSuite struct{}

var _ = gc.Suite(&runSuite{})

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
				c.Assert(result, gc.FitsTypeOf, &params.EnqueuedActions{})
				*(result.(*params.EnqueuedActions)) = params.EnqueuedActions{
					OperationTag: "operation-1",
					Actions: []params.ActionResult{{
						Action: &params.Action{
							Name:     "an action",
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
		OperationID: "1",
		Actions: []action.ActionResult{{
			Action: &action.Action{
				Name:     "an action",
				ID:       "1",
				Receiver: "machine-0",
			}}},
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
				c.Assert(result, gc.FitsTypeOf, &params.EnqueuedActions{})
				*(result.(*params.EnqueuedActions)) = params.EnqueuedActions{
					OperationTag: "operation-1",
					Actions: []params.ActionResult{{
						Action: &params.Action{
							Name:     "an action",
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
		OperationID: "1",
		Actions: []action.ActionResult{{
			Action: &action.Action{
				Name:     "an action",
				ID:       "1",
				Receiver: "machine-0",
			}}},
	})
}
