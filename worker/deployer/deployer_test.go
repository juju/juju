// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"sort"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apideployer "launchpad.net/juju-core/state/api/deployer"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/deployer"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type deployerSuite struct {
	jujutesting.JujuConnSuite
	SimpleToolsFixture

	machine       *state.Machine
	stateAPI      *api.State
	deployerState *apideployer.State
}

var _ = gc.Suite(&deployerSuite{})

var _ worker.StringsWatchHandler = (*deployer.Deployer)(nil)

func (s *deployerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.SimpleToolsFixture.SetUp(c, s.DataDir())
	s.stateAPI, s.machine = s.OpenAPIAsNewMachine(c)
	// Create the deployer facade.
	s.deployerState = s.stateAPI.Deployer()
	c.Assert(s.deployerState, gc.NotNil)
}

func (s *deployerSuite) TearDownTest(c *gc.C) {
	s.SimpleToolsFixture.TearDown(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *deployerSuite) makeDeployerAndContext(c *gc.C) (worker.Worker, deployer.Context) {
	// Create a deployer acting on behalf of the machine.
	ctx := s.getContextForMachine(c, s.machine.Tag())
	return deployer.NewDeployer(s.deployerState, ctx), ctx
}

func (s *deployerSuite) TestDeployRecallRemovePrincipals(c *gc.C) {
	// Create a machine, and a couple of units.
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	u0, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	u1, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)

	dep, ctx := s.makeDeployerAndContext(c)
	defer stop(c, dep)

	// Assign one unit, and wait for it to be deployed.
	err = u0.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)
	s.waitFor(c, isDeployed(ctx, u0.Name()))

	// Assign another unit, and wait for that to be deployed.
	err = u1.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)
	s.waitFor(c, isDeployed(ctx, u0.Name(), u1.Name()))

	// Cause a unit to become Dying, and check no change.
	err = u1.SetStatus(params.StatusInstalled, "", nil)
	c.Assert(err, gc.IsNil)
	err = u1.Destroy()
	c.Assert(err, gc.IsNil)
	s.waitFor(c, isDeployed(ctx, u0.Name(), u1.Name()))

	// Cause a unit to become Dead, and check that it is both recalled and
	// removed from state.
	err = u0.EnsureDead()
	c.Assert(err, gc.IsNil)
	s.waitFor(c, isRemoved(s.State, u0.Name()))
	s.waitFor(c, isDeployed(ctx, u1.Name()))

	// Remove the Dying unit from the machine, and check that it is recalled...
	err = u1.UnassignFromMachine()
	c.Assert(err, gc.IsNil)
	s.waitFor(c, isDeployed(ctx))

	// ...and that the deployer, no longer bearing any responsibility for the
	// Dying unit, does nothing further to it.
	err = u1.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(u1.Life(), gc.Equals, state.Dying)
}

func (s *deployerSuite) TestRemoveNonAlivePrincipals(c *gc.C) {
	// Create a service, and a couple of units.
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	u0, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	u1, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)

	// Assign the units to the machine, and set them to Dying/Dead.
	err = u0.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)
	err = u0.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = u1.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)
	// note: this is not a sane state; for the unit to have a status it must
	// have been deployed. But it's instructive to check that the right thing
	// would happen if it were possible to have a dying unit in this situation.
	err = u1.SetStatus(params.StatusInstalled, "", nil)
	c.Assert(err, gc.IsNil)
	err = u1.Destroy()
	c.Assert(err, gc.IsNil)

	// When the deployer is started, in each case (1) no unit agent is deployed
	// and (2) the non-Alive unit is been removed from state.
	dep, ctx := s.makeDeployerAndContext(c)
	defer stop(c, dep)
	s.waitFor(c, isRemoved(s.State, u0.Name()))
	s.waitFor(c, isRemoved(s.State, u1.Name()))
	s.waitFor(c, isDeployed(ctx))
}

func (s *deployerSuite) prepareSubordinates(c *gc.C) (*state.Unit, []*state.RelationUnit) {
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	u, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)
	rus := []*state.RelationUnit{}
	logging := s.AddTestingCharm(c, "logging")
	for _, name := range []string{"subsvc0", "subsvc1"} {
		s.AddTestingService(c, name, logging)
		eps, err := s.State.InferEndpoints([]string{"wordpress", name})
		c.Assert(err, gc.IsNil)
		rel, err := s.State.AddRelation(eps...)
		c.Assert(err, gc.IsNil)
		ru, err := rel.Unit(u)
		c.Assert(err, gc.IsNil)
		rus = append(rus, ru)
	}
	return u, rus
}

func (s *deployerSuite) TestDeployRecallRemoveSubordinates(c *gc.C) {
	// Create a deployer acting on behalf of the principal.
	u, rus := s.prepareSubordinates(c)
	dep, ctx := s.makeDeployerAndContext(c)
	defer stop(c, dep)

	// Add a subordinate, and wait for it to be deployed.
	err := rus[0].EnterScope(nil)
	c.Assert(err, gc.IsNil)
	sub0, err := s.State.Unit("subsvc0/0")
	c.Assert(err, gc.IsNil)
	// Make sure the principal is deployed first, then the subordinate
	s.waitFor(c, isDeployed(ctx, u.Name(), sub0.Name()))

	// And another.
	err = rus[1].EnterScope(nil)
	c.Assert(err, gc.IsNil)
	sub1, err := s.State.Unit("subsvc1/0")
	c.Assert(err, gc.IsNil)
	s.waitFor(c, isDeployed(ctx, u.Name(), sub0.Name(), sub1.Name()))

	// Set one to Dying; check nothing happens.
	err = sub1.Destroy()
	c.Assert(err, gc.IsNil)
	s.State.StartSync()
	c.Assert(isRemoved(s.State, sub1.Name())(c), gc.Equals, false)
	s.waitFor(c, isDeployed(ctx, u.Name(), sub0.Name(), sub1.Name()))

	// Set the other to Dead; check it's recalled and removed.
	err = sub0.EnsureDead()
	c.Assert(err, gc.IsNil)
	s.waitFor(c, isDeployed(ctx, u.Name(), sub1.Name()))
	s.waitFor(c, isRemoved(s.State, sub0.Name()))
}

func (s *deployerSuite) TestNonAliveSubordinates(c *gc.C) {
	// Add two subordinate units and set them to Dead/Dying respectively.
	_, rus := s.prepareSubordinates(c)
	err := rus[0].EnterScope(nil)
	c.Assert(err, gc.IsNil)
	sub0, err := s.State.Unit("subsvc0/0")
	c.Assert(err, gc.IsNil)
	err = sub0.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = rus[1].EnterScope(nil)
	c.Assert(err, gc.IsNil)
	sub1, err := s.State.Unit("subsvc1/0")
	c.Assert(err, gc.IsNil)
	err = sub1.Destroy()
	c.Assert(err, gc.IsNil)

	// When we start a new deployer, neither unit will be deployed and
	// both will be removed.
	dep, _ := s.makeDeployerAndContext(c)
	defer stop(c, dep)
	s.waitFor(c, isRemoved(s.State, sub0.Name()))
	s.waitFor(c, isRemoved(s.State, sub1.Name()))
}

func (s *deployerSuite) waitFor(c *gc.C, t func(c *gc.C) bool) {
	s.BackingState.StartSync()
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

func isDeployed(ctx deployer.Context, expected ...string) func(*gc.C) bool {
	return func(c *gc.C) bool {
		sort.Strings(expected)
		current, err := ctx.DeployedUnits()
		c.Assert(err, gc.IsNil)
		sort.Strings(current)
		return strings.Join(expected, ":") == strings.Join(current, ":")
	}
}

func isRemoved(st *state.State, name string) func(*gc.C) bool {
	return func(c *gc.C) bool {
		_, err := st.Unit(name)
		if errors.IsNotFound(err) {
			return true
		}
		c.Assert(err, gc.IsNil)
		return false
	}
}

func stop(c *gc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), gc.IsNil)
}
