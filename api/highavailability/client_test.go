// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability_test

import (
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/highavailability"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type clientSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&clientSuite{})

type KillerForTesting interface {
	KillForTesting() error
}

func assertKill(c *gc.C, killer KillerForTesting) {
	c.Assert(killer.KillForTesting(), gc.IsNil)
}

func assertEnableHA(c *gc.C, s *jujutesting.JujuConnSuite) {
	m, err := s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)

	err = m.SetMachineAddresses(
		network.NewScopedSpaceAddress("127.0.0.1", network.ScopeMachineLocal),
		network.NewScopedSpaceAddress("cloud-local0.internal", network.ScopeCloudLocal),
		network.NewScopedSpaceAddress("fc00::1", network.ScopePublic),
	)
	c.Assert(err, jc.ErrorIsNil)

	emptyCons := constraints.Value{}
	client := highavailability.NewClient(s.APIState)
	result, err := client.EnableHA(3, emptyCons, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(result.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(result.Removed, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")
	c.Assert(machines[1].Series(), gc.Equals, "quantal")
	c.Assert(machines[2].Series(), gc.Equals, "quantal")
}

func (s *clientSuite) TestClientEnableHA(c *gc.C) {
	assertEnableHA(c, &s.JujuConnSuite)
}

func (s *clientSuite) TestClientEnableHAVersion(c *gc.C) {
	client := highavailability.NewClient(s.APIState)
	c.Assert(client.BestAPIVersion(), gc.Equals, 2)
}
