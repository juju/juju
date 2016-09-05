// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/agent/engine/enginetest"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
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
	config := enginetest.AgentApiManifoldTestConfig()
	s.config = storageprovisioner.MachineManifoldConfig{
		AgentName:     config.AgentName,
		APICallerName: config.APICallerName,
		Clock:         testing.NewClock(defaultClockStart),
	}
}

func (s *MachineManifoldSuite) TestMachine(c *gc.C) {
	_, err := enginetest.RunAgentApiManifold(
		storageprovisioner.MachineManifold(s.config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		&fakeAPIConn{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.newCalled, jc.IsTrue)
}

func (s *MachineManifoldSuite) TestMissingClock(c *gc.C) {
	s.config.Clock = nil
	_, err := enginetest.RunAgentApiManifold(
		storageprovisioner.MachineManifold(s.config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		&fakeAPIConn{})
	c.Assert(err, gc.Equals, dependency.ErrMissing)
	c.Assert(s.newCalled, jc.IsFalse)
}

func (s *MachineManifoldSuite) TestUnit(c *gc.C) {
	_, err := enginetest.RunAgentApiManifold(
		storageprovisioner.MachineManifold(s.config),
		&fakeAgent{tag: names.NewUnitTag("foo/0")},
		&fakeAPIConn{})
	c.Assert(err, gc.ErrorMatches, "expected ModelTag or MachineTag, got names.UnitTag")
	c.Assert(s.newCalled, jc.IsFalse)
}

func (s *MachineManifoldSuite) TestNonAgent(c *gc.C) {
	_, err := enginetest.RunAgentApiManifold(
		storageprovisioner.MachineManifold(s.config),
		&fakeAgent{tag: names.NewUserTag("foo")},
		&fakeAPIConn{})
	c.Assert(err, gc.ErrorMatches, "expected ModelTag or MachineTag, got names.UserTag")
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
	machineJob multiwatcher.MachineJob
}

func (f *fakeAPIConn) APICall(objType string, version int, id, request string, args interface{}, response interface{}) error {
	if res, ok := response.(*params.AgentGetEntitiesResults); ok {
		res.Entities = []params.AgentGetEntitiesResult{
			{Jobs: []multiwatcher.MachineJob{f.machineJob}},
		}
	}

	return nil
}

func (*fakeAPIConn) BestFacadeVersion(facade string) int {
	return 42
}

func (f *fakeAPIConn) Agent() *apiagent.State {
	return apiagent.NewState(f)
}
