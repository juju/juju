// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/action"
	"github.com/juju/juju/rpc/params"
)

type runSuite struct{}

func TestRunSuite(t *testing.T) {
	tc.Run(t, &runSuite{})
}

func (s *actionSuite) TestRunOnAllMachines(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.RunParams{
		Commands: "pwd", Timeout: time.Millisecond}
	res := new(params.EnqueuedActions)
	ress := params.EnqueuedActions{
		OperationTag: "operation-1",
		Actions: []params.ActionResult{{
			Action: &params.Action{
				Name:     "an action",
				Tag:      "action-1",
				Receiver: "machine-0",
			},
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(
		gomock.Any(), "RunOnAllMachines", args, res,
	).DoAndReturn(func(_ context.Context, _ string, _ any, resPtr any) error {
		reflect.ValueOf(resPtr).Elem().Set(reflect.ValueOf(ress))
		return nil
	})
	client := action.NewClientFromCaller(mockFacadeCaller)

	result, err := client.RunOnAllMachines(c.Context(), "pwd", time.Millisecond)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, action.EnqueuedActions{
		OperationID: "1",
		Actions: []action.ActionResult{{
			Action: &action.Action{
				Name:     "an action",
				ID:       "1",
				Receiver: "machine-0",
			}}},
	})
}

func (s *actionSuite) TestRun(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.RunParams{
		Commands: "pwd",
		Timeout:  time.Millisecond,
		Machines: []string{"0"},
	}
	res := new(params.EnqueuedActions)
	ress := params.EnqueuedActions{
		OperationTag: "operation-1",
		Actions: []params.ActionResult{{
			Action: &params.Action{
				Name:     "an action",
				Tag:      "action-1",
				Receiver: "machine-0",
			},
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(
		gomock.Any(), "Run", args, res,
	).DoAndReturn(func(_ context.Context, _ string, _ any, resPtr any) error {
		reflect.ValueOf(resPtr).Elem().Set(reflect.ValueOf(ress))
		return nil
	})
	client := action.NewClientFromCaller(mockFacadeCaller)

	result, err := client.Run(c.Context(), action.RunParams{
		Commands: "pwd",
		Timeout:  time.Millisecond,
		Machines: []string{"0"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, action.EnqueuedActions{
		OperationID: "1",
		Actions: []action.ActionResult{{
			Action: &action.Action{
				Name:     "an action",
				ID:       "1",
				Receiver: "machine-0",
			}}},
	})
}
