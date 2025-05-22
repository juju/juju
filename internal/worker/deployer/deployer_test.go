// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/deployer"
	"github.com/juju/juju/internal/worker/deployer/mocks"
)

type deployerSuite struct {
	testhelpers.IsolationSuite
}

func TestDeployerSuite(t *testing.T) {
	tc.Run(t, &deployerSuite{})
}

func (s *deployerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	loggo.GetLogger("test.deployer").SetLogLevel(loggo.TRACE)
}

func (s *deployerSuite) sendUnitChange(c *tc.C, ch chan []string, units ...string) {
	select {
	case ch <- units:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout sending unit changes: %v", units)
	}
}

func (s *deployerSuite) TestDeployRecallRemovePrincipals(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mTag := names.NewMachineTag("666")
	ctx := &fakeContext{
		config:   agentConfig(mTag, c.MkDir(), c.MkDir()),
		deployed: set.NewStrings(),
	}
	client := mocks.NewMockClient(ctrl)
	machine := mocks.NewMockMachine(ctrl)
	client.EXPECT().Machine(mTag).Return(machine, nil)

	ch := make(chan []string)
	watch := watchertest.NewMockStringsWatcher(ch)
	machine.EXPECT().WatchUnits(gomock.Any()).Return(watch, nil)

	dep, err := deployer.NewDeployer(client, loggertesting.WrapCheckLog(c), ctx)
	c.Assert(err, tc.ErrorIsNil)
	defer stop(c, dep)

	u0 := mocks.NewMockUnit(ctrl)
	client.EXPECT().Unit(gomock.Any(), names.NewUnitTag("mysql/0")).Return(u0, nil)
	u0.EXPECT().Name().Return("mysql/0").AnyTimes()
	u0.EXPECT().Life().Return(life.Alive)
	u0.EXPECT().SetStatus(gomock.Any(), status.Waiting, status.MessageInstallingAgent, nil).Return(nil)
	u0.EXPECT().SetPassword(gomock.Any(), gomock.Any()).Return(nil)

	// Assign one unit, and wait for it to be deployed.
	s.sendUnitChange(c, ch, "mysql/0")
	s.waitFor(c, isDeployed(ctx, u0.Name()))

	// Assign another unit, and wait for that to be deployed.
	u1 := mocks.NewMockUnit(ctrl)
	client.EXPECT().Unit(gomock.Any(), names.NewUnitTag("mysql/1")).Return(u1, nil)
	u1.EXPECT().Name().Return("mysql/1").AnyTimes()
	u1.EXPECT().Life().Return(life.Alive)
	u1.EXPECT().SetStatus(gomock.Any(), status.Waiting, status.MessageInstallingAgent, nil).Return(nil)
	u1.EXPECT().SetPassword(gomock.Any(), gomock.Any()).Return(nil)
	s.sendUnitChange(c, ch, "mysql/1")
	s.waitFor(c, isDeployed(ctx, u0.Name(), u1.Name()))

	// Cause a unit to become Dying, and check no change.
	client.EXPECT().Unit(gomock.Any(), names.NewUnitTag("mysql/1")).Return(u1, nil)
	u1.EXPECT().Life().Return(life.Dying)
	s.sendUnitChange(c, ch, "mysql/1")
	s.waitFor(c, isDeployed(ctx, u0.Name(), u1.Name()))

	// Cause a unit to become Dead, and check that it is recalled and
	// removed from state.
	client.EXPECT().Unit(gomock.Any(), names.NewUnitTag("mysql/0")).Return(u0, nil)
	u0.EXPECT().Life().Return(life.Dead).Times(2)
	u0.EXPECT().Remove(gomock.Any()).Return(nil)
	s.sendUnitChange(c, ch, "mysql/0")
	s.waitFor(c, isDeployed(ctx, u1.Name()))

	// Remove the Dying unit from the machine, and check that it is recalled...
	client.EXPECT().Unit(gomock.Any(), names.NewUnitTag("mysql/1")).Return(u1, nil)
	u1.EXPECT().Life().Return(life.Dead).Times(2)
	u1.EXPECT().Remove(gomock.Any()).Return(nil)
	s.sendUnitChange(c, ch, "mysql/1")
	s.waitFor(c, isDeployed(ctx))
}

func (s *deployerSuite) TestInitialStatusMessages(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mTag := names.NewMachineTag("666")
	ctx := &fakeContext{
		config:   agentConfig(mTag, c.MkDir(), c.MkDir()),
		deployed: set.NewStrings(),
	}
	client := mocks.NewMockClient(ctrl)
	machine := mocks.NewMockMachine(ctrl)
	client.EXPECT().Machine(mTag).Return(machine, nil)

	ch := make(chan []string)
	watch := watchertest.NewMockStringsWatcher(ch)
	machine.EXPECT().WatchUnits(gomock.Any()).Return(watch, nil)

	dep, err := deployer.NewDeployer(client, loggertesting.WrapCheckLog(c), ctx)
	c.Assert(err, tc.ErrorIsNil)
	defer stop(c, dep)

	u0 := mocks.NewMockUnit(ctrl)
	client.EXPECT().Unit(gomock.Any(), names.NewUnitTag("mysql/0")).Return(u0, nil)
	u0.EXPECT().Name().Return("mysql/0").AnyTimes()
	u0.EXPECT().Life().Return(life.Alive)
	u0.EXPECT().SetStatus(gomock.Any(), status.Waiting, status.MessageInstallingAgent, nil).Return(nil)
	u0.EXPECT().SetPassword(gomock.Any(), gomock.Any()).Return(nil)

	s.sendUnitChange(c, ch, "mysql/0")
	s.waitFor(c, isDeployed(ctx, u0.Name()))
}

func (s *deployerSuite) TestRemoveNonAlivePrincipals(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mTag := names.NewMachineTag("666")
	ctx := &fakeContext{
		config:   agentConfig(mTag, c.MkDir(), c.MkDir()),
		deployed: set.NewStrings(),
	}
	client := mocks.NewMockClient(ctrl)

	machine := mocks.NewMockMachine(ctrl)
	client.EXPECT().Machine(mTag).Return(machine, nil)

	ch := make(chan []string, 1)
	watch := watchertest.NewMockStringsWatcher(ch)
	machine.EXPECT().WatchUnits(gomock.Any()).Return(watch, nil)

	// Assign a unit which is dead before the deployer starts.
	u0 := mocks.NewMockUnit(ctrl)
	client.EXPECT().Unit(gomock.Any(), names.NewUnitTag("mysql/0")).Return(u0, nil)
	u0.EXPECT().Name().Return("mysql/0").AnyTimes()
	u0.EXPECT().Life().Return(life.Dead).Times(2)

	// Assign another unit which is dying before the deployer starts.
	u1 := mocks.NewMockUnit(ctrl)
	client.EXPECT().Unit(gomock.Any(), names.NewUnitTag("mysql/1")).Return(u1, nil)
	u1.EXPECT().Name().Return("mysql/1").AnyTimes()
	u1.EXPECT().Life().Return(life.Dying).Times(2)

	// When the deployer is started, in each case (1) no unit agent is deployed
	// and (2) the non-Alive unit is been removed from state.
	dep, err := deployer.NewDeployer(client, loggertesting.WrapCheckLog(c), ctx)
	c.Assert(err, tc.ErrorIsNil)
	defer stop(c, dep)

	s.sendUnitChange(c, ch, "mysql/0", "mysql/1")

	u0.EXPECT().Remove(gomock.Any()).Return(nil)
	u1.EXPECT().Remove(gomock.Any()).Return(nil)
	s.waitFor(c, isNotDeployed(ctx, "mysql/0", "mysql/1"))

	// Deploy a different unit to give the test something to wait for.
	u2 := mocks.NewMockUnit(ctrl)
	client.EXPECT().Unit(gomock.Any(), names.NewUnitTag("mysql/2")).Return(u2, nil)
	u2.EXPECT().Name().Return("mysql/2").AnyTimes()
	u2.EXPECT().Life().Return(life.Alive)
	u2.EXPECT().SetStatus(gomock.Any(), status.Waiting, status.MessageInstallingAgent, nil).Return(nil)
	u2.EXPECT().SetPassword(gomock.Any(), gomock.Any()).Return(nil)

	s.sendUnitChange(c, ch, "mysql/2")

	s.waitFor(c, isDeployed(ctx, u2.Name()))
}

func (s *deployerSuite) waitFor(c *tc.C, t func(c *tc.C) bool) {
	if t(c) {
		return
	}
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout")
		case <-time.After(coretesting.ShortWait):
			if t(c) {
				return
			}
		}
	}
}

func isDeployed(ctx deployer.Context, expected ...string) func(*tc.C) bool {
	return func(c *tc.C) bool {
		sort.Strings(expected)
		current, err := ctx.DeployedUnits()
		c.Assert(err, tc.ErrorIsNil)
		sort.Strings(current)
		return strings.Join(expected, ":") == strings.Join(current, ":")
	}
}

func isNotDeployed(ctx deployer.Context, expected ...string) func(*tc.C) bool {
	return func(c *tc.C) bool {
		current, err := ctx.DeployedUnits()
		c.Assert(err, tc.ErrorIsNil)
		return set.NewStrings(current...).Intersection(set.NewStrings(expected...)).IsEmpty()
	}
}

func stop(c *tc.C, w worker.Worker) {
	c.Assert(workertest.CheckKill(c, w), tc.IsNil)
}

type fakeContext struct {
	deployer.Context

	config agent.Config

	deployed   set.Strings
	deployedMu sync.Mutex
}

func (c *fakeContext) Kill() {
}

func (c *fakeContext) Wait() error {
	return nil
}

func (c *fakeContext) DeployUnit(unitName, initialPassword string) error {
	c.deployedMu.Lock()
	defer c.deployedMu.Unlock()

	// Doesn't check for existence.
	c.deployed.Add(unitName)
	return nil
}

func (c *fakeContext) RecallUnit(unitName string) error {
	c.deployedMu.Lock()
	defer c.deployedMu.Unlock()

	c.deployed.Remove(unitName)
	return nil
}

func (c *fakeContext) DeployedUnits() ([]string, error) {
	c.deployedMu.Lock()
	defer c.deployedMu.Unlock()

	return c.deployed.SortedValues(), nil
}

func (c *fakeContext) AgentConfig() agent.Config {
	return c.config
}
