// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine/enginetest"
	"github.com/juju/juju/api"
	"github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/storageprovisioner"
	"github.com/juju/juju/rpc/params"
)

type MachineManifoldSuite struct {
	testhelpers.IsolationSuite
	config    storageprovisioner.MachineManifoldConfig
	newCalled bool
}

func TestMachineManifoldSuite(t *stdtesting.T) { tc.Run(t, &MachineManifoldSuite{}) }

var (
	defaultClockStart time.Time
)

func (s *MachineManifoldSuite) SetUpTest(c *tc.C) {
	s.newCalled = false
	s.PatchValue(&storageprovisioner.NewStorageProvisioner,
		func(config storageprovisioner.Config) (worker.Worker, error) {
			s.newCalled = true
			return nil, nil
		},
	)
	config := enginetest.AgentAPIManifoldTestConfig()
	s.config = storageprovisioner.MachineManifoldConfig{
		AgentName:     config.AgentName,
		APICallerName: config.APICallerName,
		Clock:         testclock.NewClock(defaultClockStart),
		Logger:        loggertesting.WrapCheckLog(c),
	}
}

func (s *MachineManifoldSuite) TestMachine(c *tc.C) {
	_, err := enginetest.RunAgentAPIManifold(
		storageprovisioner.MachineManifold(s.config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		&fakeAPIConn{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.newCalled, tc.IsTrue)
}

func (s *MachineManifoldSuite) TestMissingClock(c *tc.C) {
	s.config.Clock = nil
	_, err := enginetest.RunAgentAPIManifold(
		storageprovisioner.MachineManifold(s.config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		&fakeAPIConn{})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), tc.Equals, "missing Clock not valid")
	c.Assert(s.newCalled, tc.IsFalse)
}

func (s *MachineManifoldSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	_, err := enginetest.RunAgentAPIManifold(
		storageprovisioner.MachineManifold(s.config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		&fakeAPIConn{})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), tc.Equals, "missing Logger not valid")
	c.Assert(s.newCalled, tc.IsFalse)
}

func (s *MachineManifoldSuite) TestNonAgent(c *tc.C) {
	_, err := enginetest.RunAgentAPIManifold(
		storageprovisioner.MachineManifold(s.config),
		&fakeAgent{tag: names.NewUserTag("foo")},
		&fakeAPIConn{})
	c.Assert(err, tc.ErrorMatches, "this manifold may only be used inside a machine agent")
	c.Assert(s.newCalled, tc.IsFalse)
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

func (fakeConfig) DataDir() string {
	return "/path/to/data/dir"
}

type fakeAPIConn struct {
	api.Connection
	machineJob model.MachineJob
}

func (f *fakeAPIConn) APICall(ctx context.Context, objType string, version int, id, request string, args interface{}, response interface{}) error {
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
