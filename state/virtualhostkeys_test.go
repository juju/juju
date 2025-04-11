// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type VirtualHostKeysSuite struct {
	ConnSuite
}

var _ = gc.Suite(&VirtualHostKeysSuite{})

func (s *VirtualHostKeysSuite) TestMachineVirtualHostKey(c *gc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	key, err := s.State.MachineVirtualHostKey(machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key.HostKey(), gc.Not(gc.HasLen), 0)

	// check the same result with the info utility.
	info, err := virtualhostname.NewInfoMachineTarget(s.State.ModelUUID(), machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	key, err = s.State.HostKeyForVirtualHostname(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key.HostKey(), gc.Not(gc.HasLen), 0)

	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.MachineVirtualHostKey(machine.Id())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

// TestCAASUnitVirtualHostKey verifies that a CAAS unit has a host key when created.
func (s *VirtualHostKeysSuite) TestCAASUnitVirtualHostKey(c *gc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { _ = caasSt.Close() })

	f := factory.NewFactory(caasSt, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "ubuntu", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "ubuntu", Charm: ch, NumUnits: 1})

	unitName := "ubuntu/0"

	unitNames, err := app.UnitNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitNames, gc.HasLen, 1)

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	unit := units[0]

	key, err := caasSt.UnitVirtualHostKey(unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(key.HostKey()), gc.Matches, "(?s)-----BEGIN OPENSSH PRIVATE KEY-----\n.*")

	// check you get the same result via hostname.
	info, err := virtualhostname.NewInfoUnitTarget(caasSt.ModelUUID(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	keyViaHostname, err := caasSt.HostKeyForVirtualHostname(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key.HostKey(), gc.DeepEquals, keyViaHostname.HostKey())

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)

	_, err = caasSt.UnitVirtualHostKey(unitName)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

// TestCAASUnitVirtualHostKeyOnScale verifies that a CAAS unit has a host key when scaled.
func (s *VirtualHostKeysSuite) TestCAASUnitVirtualHostKeyOnScale(c *gc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { _ = caasSt.Close() })

	f := factory.NewFactory(caasSt, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "ubuntu", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "ubuntu", Charm: ch})

	unitName := "ubuntu/0"
	providerId := "ubuntu-0"

	// output of utils.AgentPasswordHash("juju")
	passwordHash := "v+jK3ht5NEdKeoQBfyxmlYe0"

	p := state.UpsertCAASUnitParams{
		AddUnitParams: state.AddUnitParams{
			UnitName:       &unitName,
			ProviderId:     &providerId,
			PasswordHash:   &passwordHash,
			VirtualHostKey: []byte("foo"),
		},
		OrderedScale: true,
	}

	err := app.SetScale(1, 0, true)
	c.Assert(err, jc.ErrorIsNil)

	unit, err := app.UpsertCAASUnit(p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit, gc.NotNil)

	key, err := caasSt.UnitVirtualHostKey(unit.UnitTag().Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(key.HostKey()), gc.Equals, "foo")

	// check you get the same result via hostname.
	info, err := virtualhostname.NewInfoUnitTarget(caasSt.ModelUUID(), unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	key, err = caasSt.HostKeyForVirtualHostname(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(key.HostKey()), gc.Equals, "foo")

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)

	_, err = caasSt.UnitVirtualHostKey(unit.UnitTag().Id())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *VirtualHostKeysSuite) TestIAASUnitVirtualHostKeyDoesNotExist(c *gc.C) {
	charm := s.AddTestingCharm(c, "wordpress")
	application := s.AddTestingApplication(c, "wordpress", charm)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.UnitVirtualHostKey(unit.Tag().Id())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *VirtualHostKeysSuite) TestIAASUnitWithPlacement(c *gc.C) {
	ch := state.AddTestingCharmForSeries(c, s.State, "quantal", "wordpress")
	app := s.AddTestingApplication(c, "wordpress", ch)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	id, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	key, err := s.State.MachineVirtualHostKey(m.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key.HostKey(), gc.Not(gc.HasLen), 0)
}

// TestMissingHostKeyDoesNotBlock verifies that removing
// a machine that does not have a host key won't fail.
func (s *VirtualHostKeysSuite) TestMissingHostKeyDoesNotBlock(c *gc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	key, err := s.State.MachineVirtualHostKey(machine.Id())
	c.Assert(err, jc.ErrorIsNil)

	state.RemoveVirtualHostKey(c, s.State, key)
	_, err = s.State.MachineVirtualHostKey(machine.Id())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
}
