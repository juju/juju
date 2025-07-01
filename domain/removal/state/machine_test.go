// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type machineSuite struct {
	baseSuite
}

func TestMachineSuite(t *testing.T) {
	tc.Run(t, &machineSuite{})
}

func (s *machineSuite) TestMachineExists(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.MachineExists(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)

	exists, err = st.MachineExists(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *machineSuite) TestGetMachineLifeSuccess(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetMachineLife(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Alive)

	// Set the unit to "dying" manually.
	s.advanceMachineLife(c, machineUUID, life.Dying)

	l, err = st.GetMachineLife(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Dying)
}

func (s *machineSuite) TestGetMachineLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetMachineLife(c.Context(), "some-unit-uuid")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *machineSuite) TestEnsureMachineNotAliveCascade(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	units, childMachines, err := st.EnsureMachineNotAliveCascade(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(units), tc.Equals, 1)
	c.Check(len(childMachines), tc.Equals, 0)

	// The unit should now be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM unit WHERE uuid = ?", units[0])
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)

	// The last machine had life "alive" and should now be "dying".
	row = s.DB().QueryRow("SELECT life_id FROM machine where uuid = ?", machineUUID)
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *machineSuite) TestEnsureMachineNotAliveCascadeChildMachines(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{
			AddUnitArg: applicationservice.AddUnitArg{
				Placement: instance.MustParsePlacement("lxd:0"),
			},
		})
	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 2)

	parentMachineUUID := s.getUnitMachineUUID(c, unitUUIDs[0])

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	units, childMachines, err := st.EnsureMachineNotAliveCascade(c.Context(), parentMachineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(units), tc.Equals, 2, tc.Commentf("this should return 2 units, one on the parent machine and one on the child machine"))
	c.Check(len(childMachines), tc.Equals, 1, tc.Commentf("this should return 1 child machine, the one that was created for the second unit"))

	// The unit should now be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM unit WHERE uuid = ?", units[0])
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1, tc.Commentf("unit should be dying, but got %d", lifeID))

	// The last machine had life "alive" and should now be "dying".
	row = s.DB().QueryRow("SELECT life_id FROM machine where uuid = ?", parentMachineUUID)
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1, tc.Commentf("parent machine should be dying, but got %d", lifeID))

	// The child machine should also be "dying".
	row = s.DB().QueryRow("SELECT life_id FROM machine where uuid = ?", childMachines[0])
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1, tc.Commentf("child machine should be dying, but got %d", lifeID))
}

func (s *machineSuite) TestEnsureMachineNotAliveCascadeDoesNotSetOtherUnitsToDying(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID0 := s.createIAASApplication(c, svc, "foo", applicationservice.AddIAASUnitArg{})
	machineUUID0 := s.getMachineUUIDFromApp(c, appUUID0)

	appUUID1 := s.createIAASApplication(c, svc, "bar", applicationservice.AddIAASUnitArg{})
	machineUUID1 := s.getMachineUUIDFromApp(c, appUUID1)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	units, childMachines, err := st.EnsureMachineNotAliveCascade(c.Context(), machineUUID0.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(units), tc.Equals, 1)
	c.Check(len(childMachines), tc.Equals, 0)

	// The unit should now be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM machine WHERE uuid = ?", machineUUID0)
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)

	// The last machine had life "alive" and should now be "dying".
	row = s.DB().QueryRow("SELECT life_id FROM machine where uuid = ?", machineUUID1)
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 0)
}

func (s *machineSuite) TestDeleteMachine(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceMachineLife(c, machineUUID, life.Dying)

	err := st.DeleteMachine(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The unit should be gone.
	exists, err := st.MachineExists(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *machineSuite) TestDeleteMachineNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteMachine(c.Context(), "0")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *machineSuite) getMachineUUIDFromApp(c *tc.C, appUUID application.ID) machine.UUID {
	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	return s.getUnitMachineUUID(c, unitUUID)
}
