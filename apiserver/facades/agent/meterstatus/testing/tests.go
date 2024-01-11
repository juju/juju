// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/meterstatus"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
	jujustate "github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

// TestGetMeterStatus tests unit meter status retrieval.
func TestGetMeterStatus(c *gc.C, status meterstatus.MeterStatus, unit *jujustate.Unit) {
	args := params.Entities{Entities: []params.Entity{{Tag: unit.Tag().String()}}}
	result, err := status.GetMeterStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Code, gc.Equals, "AMBER")
	c.Assert(result.Results[0].Info, gc.Equals, "not set")

	newCode := "GREEN"
	newInfo := "All is ok."

	err = unit.SetMeterStatus(newCode, newInfo)
	c.Assert(err, jc.ErrorIsNil)

	result, err = status.GetMeterStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Code, gc.DeepEquals, newCode)
	c.Assert(result.Results[0].Info, gc.DeepEquals, newInfo)
}

// TestWatchMeterStatus tests the meter status watcher functionality.
func TestWatchMeterStatus(c *gc.C, status meterstatus.MeterStatus, unit *jujustate.Unit, state *jujustate.State, resources *common.Resources) {
	c.Assert(resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: unit.UnitTag().String()},
		{Tag: "unit-foo-42"},
	}}
	result, err := status.WatchMeterStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(resources.Count(), gc.Equals, 1)
	resource := resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, resource.(jujustate.NotifyWatcher))
	wc.AssertNoChange()

	err = unit.SetMeterStatus("GREEN", "No additional information.")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}
