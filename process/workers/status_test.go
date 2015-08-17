// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers_test

import (
	// "github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/process/plugin"
	"github.com/juju/juju/process/workers"
	"github.com/juju/juju/testing"
	workertesting "github.com/juju/juju/worker/testing"
)

type statusWorkerSuite struct {
	testing.BaseSuite

	stub      *gitjujutesting.Stub
	runner    *workertesting.StubRunner
	apiClient statusWorkerAPIStub
	plugin    statusPluginStub
}

var _ = gc.Suite(&statusWorkerSuite{})

func (s *statusWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.runner = workertesting.NewStubRunner(s.stub)
	s.apiClient = statusWorkerAPIStub{stub: s.stub}
	s.plugin = statusPluginStub{stub: s.stub}
}

func (s *statusWorkerSuite) TestNewStatusWorker(c *gc.C) {
	event := process.Event{}
	w := workers.NewStatusWorker(event, s.apiClient)

	w.Kill()
	err := w.Wait()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusWorkerSuite) TestNewStatusWorkerFunc(c *gc.C) {
	s.plugin.pluginStatus = "running"
	event := process.Event{Kind: process.EventKindTracked, ID: "foo", Plugin: s.plugin}

	call := workers.NewStatusWorkerFunc(event, s.apiClient)
	err := call(nil)
	c.Assert(err, jc.ErrorIsNil)

	expectedSetProcStatusArgs := []interface{}{
		process.Status{State: "running", Blocker: "", Message: "foo is being tracked"},
		process.PluginStatus{State: "running"},
		[]string{"foo"},
	}

	s.stub.CheckCalls(c, []gitjujutesting.StubCall{
		{FuncName: "Status", Args: []interface{}{"foo"}},
		{FuncName: "SetProcessesStatus", Args: expectedSetProcStatusArgs},
	})
}

func (s *statusWorkerSuite) TestStatusWorkerTracked(c *gc.C) {
	s.plugin.pluginStatus = "running"
	event := process.Event{Kind: process.EventKindTracked, ID: "foo", Plugin: s.plugin}

	err := workers.StatusEventHandler([]process.Event{event}, s.apiClient, s.runner)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "StartWorker")
	c.Check(s.stub.Calls()[0].Args[0], gc.Equals, "foo")
}

func (s *statusWorkerSuite) TestStatusWorkerUntracked(c *gc.C) {
	s.plugin.pluginStatus = "shutting down"
	event := process.Event{Kind: process.EventKindUntracked, ID: "bar", Plugin: s.plugin}

	err := workers.StatusEventHandler([]process.Event{event}, s.apiClient, s.runner)
	c.Assert(err, jc.ErrorIsNil)

	expectedSetProcStatusArgs := []interface{}{
		process.Status{State: "stopping", Blocker: "", Message: "bar is no longer being tracked"},
		process.PluginStatus{State: "shutting down"},
		[]string{"bar"},
	}

	s.stub.CheckCalls(c, []gitjujutesting.StubCall{
		{FuncName: "Status", Args: []interface{}{"bar"}},
		{FuncName: "SetProcessesStatus", Args: expectedSetProcStatusArgs},
		{FuncName: "StopWorker", Args: []interface{}{"bar"}},
	})

}

type statusWorkerAPIStub struct {
	context.APIClient
	stub *gitjujutesting.Stub
}

func (s statusWorkerAPIStub) SetProcessesStatus(status process.Status, pluginStatus process.PluginStatus, ids ...string) error {
	s.stub.AddCall("SetProcessesStatus", status, pluginStatus, ids)
	return nil
}

type statusPluginStub struct {
	plugin.Plugin
	stub *gitjujutesting.Stub

	pluginStatus string
}

func (s statusPluginStub) Status(id string) (process.PluginStatus, error) {
	s.stub.AddCall("Status", id)
	return process.PluginStatus{State: s.pluginStatus}, nil
}
