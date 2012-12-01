package deployer_test

import (
	"sort"
	"strings"
	stdtesting "testing"
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/deployer"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type DeployerSuite struct {
	testing.JujuConnSuite
	SimpleToolsFixture
}

var _ = Suite(&DeployerSuite{})

func (s *DeployerSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.SimpleToolsFixture.SetUp(c, s.DataDir())
}

func (s *DeployerSuite) TearDownTest(c *C) {
	s.SimpleToolsFixture.TearDown(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *DeployerSuite) TestDeployRecallRemovePrincipals(c *C) {
	// Create a machine, and a service with several units.
	m, err := s.State.AddMachine(state.MachinerWorker)
	c.Assert(err, IsNil)
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	u0, err := svc.AddUnit()
	c.Assert(err, IsNil)
	u1, err := svc.AddUnit()
	c.Assert(err, IsNil)
	u2, err := svc.AddUnit()
	c.Assert(err, IsNil)
	u3, err := svc.AddUnit()
	c.Assert(err, IsNil)

	// Create a deployer acting on behalf of the machine.
	ctx := s.getContext(c, m.EntityName())
	ins := deployer.NewDeployer(s.State, ctx, m.WatchPrincipalUnits2())
	defer stop(c, ins)

	// Assign one unit, and wait for it to be deployed.
	err = u0.AssignToMachine(m)
	c.Assert(err, IsNil)
	s.waitFor(c, isDeployed(ctx, u0.Name()))

	// Assign another unit, and wait for that to be deployed.
	err = u1.AssignToMachine(m)
	c.Assert(err, IsNil)
	s.waitFor(c, isDeployed(ctx, u0.Name(), u1.Name()))

	// Cause a unit to become Dying, and check no change.
	err = u1.EnsureDying()
	c.Assert(err, IsNil)
	s.waitFor(c, isDeployed(ctx, u0.Name(), u1.Name()))

	// Cause a unit to become Dead, and check that it is both recalled and
	// removed from state.
	err = u0.EnsureDead()
	c.Assert(err, IsNil)
	s.waitFor(c, isRemoved(s.State, u0.Name()))
	s.waitFor(c, isDeployed(ctx, u1.Name()))

	// Remove the Dying unit from the machine, and check that it is recalled...
	err = u1.UnassignFromMachine()
	c.Assert(err, IsNil)
	s.waitFor(c, isDeployed(ctx))

	// ...and that the deployer, no longer bearing any responsibility for the
	// Dying unit, does nothing further to it.
	err = u1.Refresh()
	c.Assert(err, IsNil)
	c.Assert(u1.Life(), Equals, state.Dying)

	// Stop the deployer while we assign a couple of units, then set them to
	// Dying and Dead...
	stop(c, ins)
	err = u2.AssignToMachine(m)
	c.Assert(err, IsNil)
	err = u2.EnsureDead()
	c.Assert(err, IsNil)
	err = u3.AssignToMachine(m)
	c.Assert(err, IsNil)
	err = u3.EnsureDying()
	c.Assert(err, IsNil)

	// ...then restart the deployer and check that in each case (1) no unit
	// agent is deployed and (2) the non-Alive unit has been removed from
	// state.
	ins = deployer.NewDeployer(s.State, ctx, m.WatchPrincipalUnits2())
	defer stop(c, ins)
	s.waitFor(c, isRemoved(s.State, u2.Name()))
	s.waitFor(c, isRemoved(s.State, u3.Name()))
	s.waitFor(c, isDeployed(ctx))
}

func (s *DeployerSuite) TestDeployRecallRemoveSubordinates(c *C) {
	// Create a unit of a principal service, and create a subordinate service.
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	u, err := svc.AddUnit()
	sub, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)

	// Create a deployer acting on behalf of the principal.
	ctx := s.getContext(c, u.EntityName())
	ins := deployer.NewDeployer(s.State, ctx, u.WatchSubordinateUnits())
	defer stop(c, ins)

	// Add a subordinate, and wait for it to be deployed.
	sub0, err := sub.AddUnitSubordinateTo(u)
	c.Assert(err, IsNil)
	s.waitFor(c, isDeployed(ctx, sub0.Name()))

	// And another.
	sub1, err := sub.AddUnitSubordinateTo(u)
	c.Assert(err, IsNil)
	s.waitFor(c, isDeployed(ctx, sub0.Name(), sub1.Name()))

	// Set one to Dying; check nothing happens.
	err = sub1.EnsureDying()
	c.Assert(err, IsNil)
	s.State.StartSync()
	c.Assert(isRemoved(s.State, sub1.Name())(c), Equals, false)
	s.waitFor(c, isDeployed(ctx, sub0.Name(), sub1.Name()))

	// Set the other to Dead; check it's recalled and removed.
	err = sub0.EnsureDead()
	c.Assert(err, IsNil)
	s.waitFor(c, isDeployed(ctx, sub1.Name()))
	s.waitFor(c, isRemoved(s.State, sub0.Name()))

	// Stop the deployer for a bit while we add new subordinates and
	// set them to Dead/Dying respectively.
	stop(c, ins)
	sub2, err := sub.AddUnitSubordinateTo(u)
	c.Assert(err, IsNil)
	err = sub2.EnsureDead()
	c.Assert(err, IsNil)
	sub3, err := sub.AddUnitSubordinateTo(u)
	c.Assert(err, IsNil)
	err = sub3.EnsureDying()
	c.Assert(err, IsNil)

	// When we start a new deployer, neither unit will be deployed and
	// both will be removed.
	ins = deployer.NewDeployer(s.State, ctx, u.WatchSubordinateUnits())
	defer stop(c, ins)
	s.waitFor(c, isDeployed(ctx, sub1.Name()))
	s.waitFor(c, isRemoved(s.State, sub2.Name()))
	s.waitFor(c, isRemoved(s.State, sub3.Name()))
}

func (s *DeployerSuite) waitFor(c *C, t func(c *C) bool) {
	s.State.StartSync()
	if t(c) {
		return
	}
	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout")
		case <-time.After(50 * time.Millisecond):
			if t(c) {
				return
			}
		}
	}
	panic("unreachable")
}

func isDeployed(ctx deployer.Context, expected ...string) func(*C) bool {
	return func(c *C) bool {
		sort.Strings(expected)
		current, err := ctx.DeployedUnits()
		c.Assert(err, IsNil)
		sort.Strings(current)
		return strings.Join(expected, ":") == strings.Join(current, ":")
	}
}

func isRemoved(st *state.State, name string) func(*C) bool {
	return func(c *C) bool {
		_, err := st.Unit(name)
		if state.IsNotFound(err) {
			return true
		}
		c.Assert(err, IsNil)
		return false
	}
}

type stopper interface {
	Stop() error
}

func stop(c *C, stopper stopper) {
	c.Assert(stopper.Stop(), IsNil)
}
