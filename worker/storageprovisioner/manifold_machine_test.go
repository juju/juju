// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/agent/engine/enginetest"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/storageprovisioner"
)

type MachineManifoldSuite struct {
	testing.IsolationSuite
	config    storageprovisioner.MachineManifoldConfig
	newCalled bool
}

var (
	defaultClockStart time.Time
	_                 = gc.Suite(&MachineManifoldSuite{})
)

func (s *MachineManifoldSuite) SetUpTest(c *gc.C) {
	s.newCalled = false
	s.PatchValue(&storageprovisioner.NewStorageProvisioner,
		func(config storageprovisioner.Config) (worker.Worker, error) {
			s.newCalled = true
			return nil, nil
		},
	)
	config := enginetest.AgentAPIManifoldTestConfig()
	s.config = storageprovisioner.MachineManifoldConfig{
		AgentName:                    config.AgentName,
		APICallerName:                config.APICallerName,
		Clock:                        testclock.NewClock(defaultClockStart),
		Logger:                       loggo.GetLogger("test"),
		NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
	}
}

func (s *MachineManifoldSuite) TestMachine(c *gc.C) {
	_, err := enginetest.RunAgentAPIManifold(
		storageprovisioner.MachineManifold(s.config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		&fakeAPIConn{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.newCalled, jc.IsTrue)
}

func (s *MachineManifoldSuite) TestMissingClock(c *gc.C) {
	s.config.Clock = nil
	_, err := enginetest.RunAgentAPIManifold(
		storageprovisioner.MachineManifold(s.config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		&fakeAPIConn{})
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), gc.Equals, "missing Clock not valid")
	c.Assert(s.newCalled, jc.IsFalse)
}

func (s *MachineManifoldSuite) TestMissingLogger(c *gc.C) {
	s.config.Logger = nil
	_, err := enginetest.RunAgentAPIManifold(
		storageprovisioner.MachineManifold(s.config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		&fakeAPIConn{})
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), gc.Equals, "missing Logger not valid")
	c.Assert(s.newCalled, jc.IsFalse)
}

func (s *MachineManifoldSuite) TestNonAgent(c *gc.C) {
	_, err := enginetest.RunAgentAPIManifold(
		storageprovisioner.MachineManifold(s.config),
		&fakeAgent{tag: names.NewUserTag("foo")},
		&fakeAPIConn{})
	c.Assert(err, gc.ErrorMatches, "this manifold may only be used inside a machine agent")
	c.Assert(s.newCalled, jc.IsFalse)
}

type fakeAgent struct {
	agent.Agent
	tag names.Tag
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{tag: a.tag}
}

type fakeConfig struct {
	agent.Config
	tag names.Tag
}

func (c *fakeConfig) Tag() names.Tag {
	return c.tag
}

func (_ fakeConfig) DataDir() string {
	return "/path/to/data/dir"
}

type fakeAPIConn struct {
	api.Connection
	machineJob model.MachineJob
}

func (f *fakeAPIConn) APICall(objType string, version int, id, request string, args interface{}, response interface{}) error {
	if res, ok := response.(*params.AgentGetEntitiesResults); ok {
		res.Entities = []params.AgentGetEntitiesResult{
			{Jobs: []model.MachineJob{f.machineJob}},
		}
	}

	return nil
}

func (*fakeAPIConn) BestFacadeVersion(facade string) int {
	return 42
}
