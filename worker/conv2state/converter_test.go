// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package conv2state

import (
	stderrs "errors"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&Suite{})

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type Suite struct {
	coretesting.BaseSuite
}

func (Suite) TestSetUp(c *gc.C) {
	a := &fakeAgent{tag: names.NewMachineTag("1")}
	m := &fakeMachine{}
	mr := &fakeMachiner{m: m}
	conv := converter{machiner: mr, agent: a}
	w, err := conv.SetUp()
	c.Assert(err, gc.IsNil)
	c.Assert(conv.machine, gc.Equals, m)
	c.Assert(mr.gotTag, gc.Equals, a.tag.(names.MachineTag))
	c.Assert(w, gc.Equals, m.w)
}

func (Suite) TestSetupMachinerErr(c *gc.C) {
	a := &fakeAgent{tag: names.NewMachineTag("1")}
	mr := &fakeMachiner{err: stderrs.New("foo")}
	conv := converter{machiner: mr, agent: a}
	w, err := conv.SetUp()
	c.Assert(errors.Cause(err), gc.Equals, mr.err)
	c.Assert(mr.gotTag, gc.Equals, a.tag.(names.MachineTag))
	c.Assert(w, gc.IsNil)
}

func (Suite) TestSetupWatchErr(c *gc.C) {
	a := &fakeAgent{tag: names.NewMachineTag("1")}
	m := &fakeMachine{watchErr: stderrs.New("foo")}
	mr := &fakeMachiner{m: m}
	conv := &converter{machiner: mr, agent: a}
	w, err := conv.SetUp()
	c.Assert(errors.Cause(err), gc.Equals, m.watchErr)
	c.Assert(mr.gotTag, gc.Equals, a.tag.(names.MachineTag))
	c.Assert(w, gc.IsNil)
}

func (s Suite) TestHandle(c *gc.C) {
	a := &fakeAgent{tag: names.NewMachineTag("1")}
	jobs := []model.MachineJob{model.JobHostUnits, model.JobManageModel}
	m := &fakeMachine{
		jobs: &params.JobsResult{Jobs: jobs},
	}
	mr := &fakeMachiner{m: m}
	conv := &converter{machiner: mr, agent: a}
	_, err := conv.SetUp()
	c.Assert(err, gc.IsNil)
	err = conv.Handle(nil)
	// Since machine has model.JobManageModel, we expect an error
	// which will get agent to restart.
	c.Assert(err.Error(), gc.Equals, "bounce agent to pick up new jobs")
}

func (s Suite) TestHandleNoManageEnviron(c *gc.C) {
	a := &fakeAgent{tag: names.NewMachineTag("1")}
	jobs := []model.MachineJob{model.JobHostUnits}
	m := &fakeMachine{
		jobs: &params.JobsResult{Jobs: jobs},
	}
	mr := &fakeMachiner{m: m}
	conv := &converter{machiner: mr, agent: a}
	_, err := conv.SetUp()
	c.Assert(err, gc.IsNil)
	err = conv.Handle(nil)
	c.Assert(err, gc.IsNil)
}

func (Suite) TestHandleJobsError(c *gc.C) {
	a := &fakeAgent{tag: names.NewMachineTag("1")}
	jobs := []model.MachineJob{model.JobHostUnits, model.JobManageModel}
	m := &fakeMachine{
		jobs:    &params.JobsResult{Jobs: jobs},
		jobsErr: errors.New("foo"),
	}
	mr := &fakeMachiner{m: m}
	conv := &converter{machiner: mr, agent: a}
	_, err := conv.SetUp()
	c.Assert(err, gc.IsNil)
	err = conv.Handle(nil)
	c.Assert(errors.Cause(err), gc.Equals, m.jobsErr)
}

func (s Suite) TestHandleRestartError(c *gc.C) {
	a := &fakeAgent{
		tag: names.NewMachineTag("1"),
	}
	jobs := []model.MachineJob{model.JobHostUnits, model.JobManageModel}
	m := &fakeMachine{
		jobs: &params.JobsResult{Jobs: jobs},
	}
	mr := &fakeMachiner{m: m}
	conv := &converter{machiner: mr, agent: a}
	_, err := conv.SetUp()
	c.Assert(err, gc.IsNil)
	err = conv.Handle(nil)
	c.Assert(err.Error(), gc.Equals, "bounce agent to pick up new jobs")
}
