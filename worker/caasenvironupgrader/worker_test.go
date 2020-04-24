// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasenvironupgrader_test

import (
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasenvironupgrader"
)

type WorkerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&WorkerSuite{})

func (*WorkerSuite) TestNewWorkerValidatesConfig(c *gc.C) {
	_, err := caasenvironupgrader.NewWorker(caasenvironupgrader.Config{})
	c.Assert(err, gc.ErrorMatches, "nil Facade not valid")
}

func (*WorkerSuite) TestNewWorker(c *gc.C) {
	mockFacade := mockFacade{}
	mockGateUnlocker := mockGateUnlocker{}
	w, err := caasenvironupgrader.NewWorker(caasenvironupgrader.Config{
		Facade:       &mockFacade,
		GateUnlocker: &mockGateUnlocker,
		ModelTag:     coretesting.ModelTag,
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckKill(c, w)
	mockFacade.CheckCalls(c, []testing.StubCall{
		{"SetModelStatus", []interface{}{coretesting.ModelTag, status.Available, "", nilData}},
	})
	mockGateUnlocker.CheckCallNames(c, "Unlock")
}

type mockFacade struct {
	testing.Stub
}

var nilData map[string]interface{}

func (f *mockFacade) SetModelStatus(tag names.ModelTag, status status.Status, info string, data map[string]interface{}) error {
	f.MethodCall(f, "SetModelStatus", tag, status, info, data)
	return f.NextErr()
}

type mockGateUnlocker struct {
	testing.Stub
}

func (g *mockGateUnlocker) Unlock() {
	g.MethodCall(g, "Unlock")
	g.PopNoErr()
}
