// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/charmrepo"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

// DeployLocalSuite uses a fresh copy of the same local dummy charm for each
// test, because DeployService demands that a charm already exists in state,
// and that's is the simplest way to get one in there.
type DeployLocalSuite struct {
	testing.JujuConnSuite
	repo        charmrepo.Interface
	charm       *state.Charm
	oldCacheDir string
}

var _ = gc.Suite(&DeployLocalSuite{})

func (s *DeployLocalSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.repo = &charmrepo.LocalRepository{Path: testcharms.Repo.Path()}
	s.oldCacheDir, charmrepo.CacheDir = charmrepo.CacheDir, c.MkDir()
}

func (s *DeployLocalSuite) TearDownSuite(c *gc.C) {
	charmrepo.CacheDir = s.oldCacheDir
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *DeployLocalSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	curl := charm.MustParseURL("local:quantal/dummy")
	charm, err := testing.PutCharm(s.State, curl, s.repo, false)
	c.Assert(err, jc.ErrorIsNil)
	s.charm = charm
}

func (s *DeployLocalSuite) TestDeployMinimal(c *gc.C) {
	service, err := juju.DeployService(s.State,
		juju.DeployServiceParams{
			ServiceName: "bob",
			Charm:       s.charm,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharm(c, service, s.charm.URL())
	s.assertSettings(c, service, charm.Settings{})
	s.assertConstraints(c, service, constraints.Value{})
	s.assertMachines(c, service, constraints.Value{})
	c.Assert(service.GetOwnerTag(), gc.Equals, s.AdminUserTag(c).String())
}

func (s *DeployLocalSuite) TestDeployOwnerTag(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})
	service, err := juju.DeployService(s.State,
		juju.DeployServiceParams{
			ServiceName:  "bobwithowner",
			Charm:        s.charm,
			ServiceOwner: "user-foobar",
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(service.GetOwnerTag(), gc.Equals, "user-foobar")
}

func (s *DeployLocalSuite) TestDeploySettings(c *gc.C) {
	service, err := juju.DeployService(s.State,
		juju.DeployServiceParams{
			ServiceName: "bob",
			Charm:       s.charm,
			ConfigSettings: charm.Settings{
				"title":       "banana cupcakes",
				"skill-level": 9901,
			},
		})
	c.Assert(err, jc.ErrorIsNil)
	s.assertSettings(c, service, charm.Settings{
		"title":       "banana cupcakes",
		"skill-level": int64(9901),
	})
}

func (s *DeployLocalSuite) TestDeploySettingsError(c *gc.C) {
	_, err := juju.DeployService(s.State,
		juju.DeployServiceParams{
			ServiceName: "bob",
			Charm:       s.charm,
			ConfigSettings: charm.Settings{
				"skill-level": 99.01,
			},
		})
	c.Assert(err, gc.ErrorMatches, `option "skill-level" expected int, got 99.01`)
	_, err = s.State.Service("bob")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DeployLocalSuite) TestDeployConstraints(c *gc.C) {
	err := s.State.SetEnvironConstraints(constraints.MustParse("mem=2G"))
	c.Assert(err, jc.ErrorIsNil)
	serviceCons := constraints.MustParse("cpu-cores=2")
	service, err := juju.DeployService(s.State,
		juju.DeployServiceParams{
			ServiceName: "bob",
			Charm:       s.charm,
			Constraints: serviceCons,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.assertConstraints(c, service, serviceCons)
}

func (s *DeployLocalSuite) TestDeployNumUnits(c *gc.C) {
	err := s.State.SetEnvironConstraints(constraints.MustParse("mem=2G"))
	c.Assert(err, jc.ErrorIsNil)
	serviceCons := constraints.MustParse("cpu-cores=2")
	service, err := juju.DeployService(s.State,
		juju.DeployServiceParams{
			ServiceName: "bob",
			Charm:       s.charm,
			Constraints: serviceCons,
			NumUnits:    2,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.assertConstraints(c, service, serviceCons)
	s.assertMachines(c, service, constraints.MustParse("mem=2G cpu-cores=2"), "0", "1")
}

func (s *DeployLocalSuite) TestDeployWithForceMachineRejectsTooManyUnits(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, "0")
	_, err = juju.DeployService(s.State,
		juju.DeployServiceParams{
			ServiceName:   "bob",
			Charm:         s.charm,
			NumUnits:      2,
			ToMachineSpec: "0",
		})
	c.Assert(err, gc.ErrorMatches, "cannot use --num-units with --to")
}

func (s *DeployLocalSuite) TestDeployForceMachineId(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, "0")
	err = s.State.SetEnvironConstraints(constraints.MustParse("mem=2G"))
	c.Assert(err, jc.ErrorIsNil)
	serviceCons := constraints.MustParse("cpu-cores=2")
	service, err := juju.DeployService(s.State,
		juju.DeployServiceParams{
			ServiceName:   "bob",
			Charm:         s.charm,
			Constraints:   serviceCons,
			NumUnits:      1,
			ToMachineSpec: "0",
		})
	c.Assert(err, jc.ErrorIsNil)
	s.assertConstraints(c, service, serviceCons)
	s.assertMachines(c, service, constraints.Value{}, "0")
}

func (s *DeployLocalSuite) TestDeployForceMachineIdWithContainer(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, "0")
	envCons := constraints.MustParse("mem=2G")
	err = s.State.SetEnvironConstraints(envCons)
	c.Assert(err, jc.ErrorIsNil)
	serviceCons := constraints.MustParse("cpu-cores=2")
	service, err := juju.DeployService(s.State,
		juju.DeployServiceParams{
			ServiceName:   "bob",
			Charm:         s.charm,
			Constraints:   serviceCons,
			NumUnits:      1,
			ToMachineSpec: fmt.Sprintf("%s:0", instance.LXC),
		})
	c.Assert(err, jc.ErrorIsNil)
	s.assertConstraints(c, service, serviceCons)
	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	// The newly created container will use the constraints.
	id, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err = s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	machineCons, err := machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	unitCons, err := units[0].Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineCons, gc.DeepEquals, *unitCons)
}

func (s *DeployLocalSuite) TestDeployWithPlacement(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, "0")
	err = s.State.SetEnvironConstraints(constraints.MustParse("mem=2G"))
	c.Assert(err, jc.ErrorIsNil)
	serviceCons := constraints.MustParse("cpu-cores=2")
	service, err := juju.DeployService(s.State,
		juju.DeployServiceParams{
			ServiceName: "bob",
			Charm:       s.charm,
			Constraints: serviceCons,
			NumUnits:    3,
			Placement: []*instance.Placement{
				{Scope: s.State.EnvironUUID(), Directive: "valid"},
				{Scope: "#", Directive: "0"},
				{Scope: "lxc", Directive: "1"},
			},
			ToMachineSpec: "will be ignored",
		})
	c.Assert(err, jc.ErrorIsNil)
	s.assertConstraints(c, service, serviceCons)
	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 3)

	// Check each of the newly added units.
	s.assertAssignedUnit(c, units[0], "1", constraints.MustParse("mem=2G cpu-cores=2"))
	s.assertAssignedUnit(c, units[1], "0", constraints.Value{})
	s.assertAssignedUnit(c, units[2], "1/lxc/0", constraints.MustParse("mem=2G cpu-cores=2"))
}

func (s *DeployLocalSuite) TestDeployWithFewerPlacement(c *gc.C) {
	err := s.State.SetEnvironConstraints(constraints.MustParse("mem=2G"))
	c.Assert(err, jc.ErrorIsNil)
	serviceCons := constraints.MustParse("cpu-cores=2")
	service, err := juju.DeployService(s.State,
		juju.DeployServiceParams{
			ServiceName: "bob",
			Charm:       s.charm,
			Constraints: serviceCons,
			NumUnits:    3,
			Placement: []*instance.Placement{
				{Scope: s.State.EnvironUUID(), Directive: "valid"},
			},
		})
	c.Assert(err, jc.ErrorIsNil)
	s.assertConstraints(c, service, serviceCons)
	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 3)

	// Check each of the newly added units.
	s.assertAssignedUnit(c, units[0], "0", constraints.MustParse("mem=2G cpu-cores=2"))
	s.assertAssignedUnit(c, units[1], "1", constraints.MustParse("mem=2G cpu-cores=2"))
	s.assertAssignedUnit(c, units[2], "2", constraints.MustParse("mem=2G cpu-cores=2"))
}

func (s *DeployLocalSuite) assertAssignedUnit(c *gc.C, u *state.Unit, mId string, cons constraints.Value) {
	id, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	machineCons, err := machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineCons, gc.DeepEquals, cons)
}

func (s *DeployLocalSuite) assertCharm(c *gc.C, service *state.Service, expect *charm.URL) {
	curl, force := service.CharmURL()
	c.Assert(curl, gc.DeepEquals, expect)
	c.Assert(force, jc.IsFalse)
}

func (s *DeployLocalSuite) assertSettings(c *gc.C, service *state.Service, expect charm.Settings) {
	settings, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, expect)
}

func (s *DeployLocalSuite) assertConstraints(c *gc.C, service *state.Service, expect constraints.Value) {
	cons, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, expect)
}

func (s *DeployLocalSuite) assertMachines(c *gc.C, service *state.Service, expectCons constraints.Value, expectIds ...string) {
	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, len(expectIds))
	unseenIds := set.NewStrings(expectIds...)
	for _, unit := range units {
		id, err := unit.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		unseenIds.Remove(id)
		machine, err := s.State.Machine(id)
		c.Assert(err, jc.ErrorIsNil)
		cons, err := machine.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cons, gc.DeepEquals, expectCons)
	}
	c.Assert(unseenIds, gc.DeepEquals, set.NewStrings())
}
