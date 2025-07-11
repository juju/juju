// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	removalerrors "github.com/juju/juju/domain/removal/errors"
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
	svc := s.setupApplicationService(c, factory)
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
	svc := s.setupApplicationService(c, factory)
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

func (s *machineSuite) TestGetInstanceLifeSuccess(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetInstanceLife(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Alive)

	// Set the unit to "dying" manually.
	s.advanceInstanceLife(c, machineUUID, life.Dying)

	l, err = st.GetInstanceLife(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Dying)
}

func (s *machineSuite) TestGetInstanceLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetInstanceLife(c.Context(), "some-unit-uuid")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *machineSuite) TestGetMachineNetworkInterfacesNoHardwareDevices(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	interfaces, err := st.GetMachineNetworkInterfaces(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(interfaces), tc.Equals, 0)
}

func (s *machineSuite) TestGetMachineNetworkInterfaces(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var netNodeUUID string
		err := s.DB().QueryRowContext(ctx, `
SELECT net_node_uuid FROM machine WHERE uuid = ?`, machineUUID.String()).Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		_, err = s.DB().ExecContext(ctx, `
INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) 
VALUES ('abc', ?, ?, ?, ?, ?, ?)`, netNodeUUID, "lld-name", 1500, "00:11:22:33:44:55", 0, 0)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceMachineLife(c, machineUUID, life.Dying)

	interfaces, err := st.GetMachineNetworkInterfaces(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(interfaces), tc.Equals, 0)
}

func (s *machineSuite) TestGetMachineNetworkInterfacesMultiple(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var netNodeUUID string
		err := s.DB().QueryRowContext(ctx, `
SELECT net_node_uuid FROM machine WHERE uuid = ?`, machineUUID.String()).Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		_, err = s.DB().ExecContext(ctx, `
INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) 
VALUES ('abc', ?, ?, ?, ?, ?, ?)`, netNodeUUID, "lld-name", 1500, "00:11:22:33:44:55", 0, 0)
		if err != nil {
			return err
		}
		_, err = s.DB().ExecContext(ctx, `
INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) 
VALUES ('def', ?, ?, ?, ?, ?, ?)`, netNodeUUID, "lld-name", 1500, "66:11:22:33:44:56", 0, 0)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceMachineLife(c, machineUUID, life.Dying)

	interfaces, err := st.GetMachineNetworkInterfaces(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(interfaces), tc.Equals, 0)
}

func (s *machineSuite) TestGetMachineNetworkInterfacesContainer(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID0 := s.createIAASApplication(c, svc, "some-app1", applicationservice.AddIAASUnitArg{})
	appUUID1 := s.createIAASApplication(c, svc, "some-app2", applicationservice.AddIAASUnitArg{
		AddUnitArg: applicationservice.AddUnitArg{
			Placement: instance.MustParsePlacement("lxd:0"),
		},
	})
	machineUUID0 := s.getMachineUUIDFromApp(c, appUUID0)
	machineUUID1 := s.getMachineUUIDFromApp(c, appUUID1)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var netNodeUUID string
		err := s.DB().QueryRowContext(ctx, `
SELECT net_node_uuid FROM machine WHERE uuid = ?`, machineUUID0.String()).Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		_, err = s.DB().ExecContext(ctx, `
INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) 
VALUES ('abc', ?, ?, ?, ?, ?, ?)`, netNodeUUID, "lld-name-0", 1500, "00:11:22:33:44:55", 0, 0)
		if err != nil {
			return err
		}

		err = s.DB().QueryRowContext(ctx, `
SELECT net_node_uuid FROM machine WHERE uuid = ?`, machineUUID1.String()).Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		_, err = s.DB().ExecContext(ctx, `
INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) 
VALUES ('def', ?, ?, ?, ?, ?, ?)`, netNodeUUID, "lld-name-1", 1500, "11:11:22:33:44:66", 0, 0)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceMachineLife(c, machineUUID0, life.Dying)
	s.advanceMachineLife(c, machineUUID1, life.Dying)

	interfaces, err := st.GetMachineNetworkInterfaces(c.Context(), machineUUID0.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(interfaces), tc.Equals, 0)

	interfaces, err = st.GetMachineNetworkInterfaces(c.Context(), machineUUID1.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(interfaces), tc.Equals, 1)
	c.Check(interfaces, tc.DeepEquals, []string{"11:11:22:33:44:66"})
}

func (s *machineSuite) TestEnsureMachineNotAliveCascade(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	units, childMachines, err := st.EnsureMachineNotAliveCascade(c.Context(), machineUUID.String(), true)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(units), tc.Equals, 1)
	c.Check(len(childMachines), tc.Equals, 0)

	s.checkUnitLife(c, units[0], 1)
	s.checkMachineLife(c, machineUUID.String(), 1)
	s.checkInstanceLife(c, machineUUID.String(), 1)
}

func (s *machineSuite) TestEnsureMachineNotAliveCascadeCoHostedUnits(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{
			AddUnitArg: applicationservice.AddUnitArg{
				Placement: instance.MustParsePlacement("0"),
			},
		})
	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 2)

	parentMachineUUID := s.getUnitMachineUUID(c, unitUUIDs[0])

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	units, childMachines, err := st.EnsureMachineNotAliveCascade(c.Context(), parentMachineUUID.String(), true)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(units), tc.Equals, 2)
	c.Check(len(childMachines), tc.Equals, 0)

	// The unit should now be "dying".
	s.checkUnitLife(c, units[0], 1)
	s.checkUnitLife(c, units[1], 1)

	// The last machine had life "alive" and should now be "dying".
	s.checkMachineLife(c, parentMachineUUID.String(), 1)
	s.checkInstanceLife(c, parentMachineUUID.String(), 1)
}

func (s *machineSuite) TestEnsureMachineNotAliveCascadeChildMachines(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
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

	units, childMachines, err := st.EnsureMachineNotAliveCascade(c.Context(), parentMachineUUID.String(), true)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(units), tc.Equals, 2, tc.Commentf("this should return 2 units, one on the parent machine and one on the child machine"))
	c.Check(len(childMachines), tc.Equals, 1, tc.Commentf("this should return 1 child machine, the one that was created for the second unit"))

	s.checkUnitLife(c, units[0], 1)
	s.checkUnitLife(c, units[1], 1)

	// The last machine had life "alive" and should now be "dying".
	s.checkMachineLife(c, parentMachineUUID.String(), 1)
	s.checkMachineLife(c, childMachines[0], 1)

	s.checkInstanceLife(c, parentMachineUUID.String(), 1)
	s.checkInstanceLife(c, childMachines[0], 1)
}

func (s *machineSuite) TestEnsureMachineNotAliveCascadeDoesNotSetOtherUnitsToDying(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID0 := s.createIAASApplication(c, svc, "foo", applicationservice.AddIAASUnitArg{})
	machineUUID0 := s.getMachineUUIDFromApp(c, appUUID0)

	appUUID1 := s.createIAASApplication(c, svc, "bar", applicationservice.AddIAASUnitArg{})
	machineUUID1 := s.getMachineUUIDFromApp(c, appUUID1)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	units, childMachines, err := st.EnsureMachineNotAliveCascade(c.Context(), machineUUID0.String(), true)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(units), tc.Equals, 1)
	c.Check(len(childMachines), tc.Equals, 0)

	s.checkMachineLife(c, machineUUID0.String(), 1)
	s.checkInstanceLife(c, machineUUID0.String(), 1)

	// The other machine should not be affected.
	s.checkMachineLife(c, machineUUID1.String(), 0)
	s.checkInstanceLife(c, machineUUID1.String(), 0)
}

func (s *machineSuite) TestEnsureMachineNotAliveCasscadeWithoutForceSucceedsForEmptyMachine(c *tc.C) {
	svc := s.setupMachineService(c)
	res, err := svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	uuid, err := svc.GetMachineUUID(c.Context(), res.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	units, childMachines, err := st.EnsureMachineNotAliveCascade(c.Context(), uuid.String(), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 0)
	c.Assert(childMachines, tc.HasLen, 0)

	s.checkMachineLife(c, uuid.String(), 1)
	s.checkInstanceLife(c, uuid.String(), 1)
}

func (s *machineSuite) TestEnsureMachineNotAliveCascadeWithoutForceFailsForMachineHostingContainer(c *tc.C) {
	svc := s.setupMachineService(c)
	machineRes, err := svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	containerRes, err := svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
			Directive: machineRes.MachineName.String(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	machineUUID, err := svc.GetMachineUUID(c.Context(), machineRes.MachineName)
	c.Assert(err, tc.ErrorIsNil)
	containerUUID, err := svc.GetMachineUUID(c.Context(), containerRes.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, _, err = st.EnsureMachineNotAliveCascade(c.Context(), machineUUID.String(), false)
	c.Assert(err, tc.ErrorIs, removalerrors.MachineHasContainers)

	s.checkMachineLife(c, machineUUID.String(), 0)
	s.checkInstanceLife(c, machineUUID.String(), 0)
	s.checkMachineLife(c, containerUUID.String(), 0)
	s.checkInstanceLife(c, containerUUID.String(), 0)
}

func (s *machineSuite) TestEnsureMachineNotAliveCascadeWithoutForceFailsForMachineHostingUnits(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "foo", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, _, err := st.EnsureMachineNotAliveCascade(c.Context(), machineUUID.String(), false)
	c.Assert(err, tc.ErrorIs, removalerrors.MachineHasUnits)

	s.checkMachineLife(c, machineUUID.String(), 0)
	s.checkInstanceLife(c, machineUUID.String(), 0)
}

func (s *machineSuite) TestMachineRemovalNormalSuccess(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.MachineScheduleRemoval(
		c.Context(), "removal-uuid", machineUUID.String(), false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	row := s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal where uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalTypeID, tc.Equals, 3)
	c.Check(rUUID, tc.Equals, machineUUID.String())
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *machineSuite) TestMachineRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.MachineScheduleRemoval(
		c.Context(), "removal-uuid", "some-machine-uuid", true, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	// It doesn't matter that the machine does not exist.
	// We rely on the worker to handle that fact.
	row := s.DB().QueryRow(`
SELECT t.name, r.entity_uuid, r.force, r.scheduled_for 
FROM   removal r JOIN removal_type t ON r.removal_type_id = t.id
where  r.uuid = ?`, "removal-uuid",
	)

	var (
		removalType  string
		rUUID        string
		force        bool
		scheduledFor time.Time
	)
	err = row.Scan(&removalType, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalType, tc.Equals, "machine")
	c.Check(rUUID, tc.Equals, "some-machine-uuid")
	c.Check(force, tc.Equals, true)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *machineSuite) TestMarkMachineAsDead(c *tc.C) {
	svc := s.setupMachineService(c)
	machineRes, err := svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := svc.GetMachineUUID(c.Context(), machineRes.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.MarkMachineAsDead(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)

	s.advanceMachineLife(c, machineUUID, life.Dying)

	err = st.MarkMachineAsDead(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The machine should now be dead.
	s.checkMachineLife(c, machineUUID.String(), 2)
}

func (s *machineSuite) TestMarkMachineAsDeadNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.MarkMachineAsDead(c.Context(), "abc")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *machineSuite) TestMarkMachineAsDeadMachineHasContainers(c *tc.C) {
	svc := s.setupMachineService(c)
	machineRes, err := svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := svc.GetMachineUUID(c.Context(), machineRes.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	_, err = svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
			Directive: machineRes.MachineName.String(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceMachineLife(c, machineUUID, life.Dying)

	err = st.MarkMachineAsDead(c.Context(), machineUUID.String())
	c.Check(err, tc.ErrorIs, removalerrors.MachineHasContainers)

	s.checkMachineLife(c, machineUUID.String(), 1)
}

func (s *machineSuite) TestMarkMachineAsDeadMachineHasUnits(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceMachineLife(c, machineUUID, life.Dying)

	err := st.MarkMachineAsDead(c.Context(), machineUUID.String())
	c.Check(err, tc.ErrorIs, removalerrors.MachineHasUnits)

	s.checkMachineLife(c, machineUUID.String(), 1)
}

func (s *machineSuite) TestMarkInstanceAsDead(c *tc.C) {
	svc := s.setupMachineService(c)
	machineRes, err := svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := svc.GetMachineUUID(c.Context(), machineRes.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceInstanceLife(c, machineUUID, life.Dying)

	err = st.MarkInstanceAsDead(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	s.checkInstanceLife(c, machineUUID.String(), 2)
}

func (s *machineSuite) TestMarkInstanceAsDeadNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.MarkInstanceAsDead(c.Context(), "abc")
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *machineSuite) TestMarkInstanceAsDeadMachineHasContainers(c *tc.C) {
	svc := s.setupMachineService(c)
	machineRes, err := svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := svc.GetMachineUUID(c.Context(), machineRes.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	_, err = svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
			Directive: machineRes.MachineName.String(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceInstanceLife(c, machineUUID, life.Dying)

	err = st.MarkInstanceAsDead(c.Context(), machineUUID.String())
	c.Check(err, tc.ErrorIs, removalerrors.MachineHasContainers)

	s.checkInstanceLife(c, machineUUID.String(), 1)
}

func (s *machineSuite) TestMarkInstanceAsDeadMachineHasUnits(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceInstanceLife(c, machineUUID, life.Dying)

	err := st.MarkInstanceAsDead(c.Context(), machineUUID.String())
	c.Check(err, tc.ErrorIs, removalerrors.MachineHasUnits)

	s.checkInstanceLife(c, machineUUID.String(), 1)
}

func (s *machineSuite) TestDeleteMachine(c *tc.C) {
	svc := s.setupMachineService(c)
	machineRes, err := svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := svc.GetMachineUUID(c.Context(), machineRes.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceMachineLife(c, machineUUID, life.Dead)
	s.advanceInstanceLife(c, machineUUID, life.Dead)

	err = st.DeleteMachine(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The machine should be gone.
	exists, err := st.MachineExists(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *machineSuite) TestDeleteMachineNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteMachine(c.Context(), "0")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *machineSuite) TestDeleteMachineDying(c *tc.C) {
	svc := s.setupMachineService(c)
	machineRes, err := svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := svc.GetMachineUUID(c.Context(), machineRes.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceMachineLife(c, machineUUID, life.Dying)
	s.advanceInstanceLife(c, machineUUID, life.Dead)

	err = st.DeleteMachine(c.Context(), machineUUID.String())
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)

	// The machine should not be gone.
	exists, err := st.MachineExists(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

func (s *machineSuite) TestDeleteMachineInstanceDying(c *tc.C) {
	svc := s.setupMachineService(c)
	machineRes, err := svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := svc.GetMachineUUID(c.Context(), machineRes.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceMachineLife(c, machineUUID, life.Dead)
	s.advanceInstanceLife(c, machineUUID, life.Dying)

	err = st.DeleteMachine(c.Context(), machineUUID.String())
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)

	// The machine should not be gone.
	exists, err := st.MachineExists(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

func (s *machineSuite) TestDeleteMachineWithContainers(c *tc.C) {
	svc := s.setupMachineService(c)
	machineRes, err := svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := svc.GetMachineUUID(c.Context(), machineRes.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	containerRes, err := svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
			Directive: machineRes.MachineName.String(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	containerUUID, err := svc.GetMachineUUID(c.Context(), containerRes.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceMachineLife(c, machineUUID, life.Dead)
	s.advanceInstanceLife(c, machineUUID, life.Dead)
	s.advanceMachineLife(c, containerUUID, life.Dead)
	s.advanceInstanceLife(c, containerUUID, life.Dead)

	err = st.DeleteMachine(c.Context(), machineUUID.String())
	c.Check(err, tc.ErrorIs, removalerrors.MachineHasContainers)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)

	// The machine should not be gone.
	exists, err := st.MachineExists(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

func (s *machineSuite) TestDeleteMachineWithUnits(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.advanceMachineLife(c, machineUUID, life.Dead)
	s.advanceInstanceLife(c, machineUUID, life.Dead)

	err := st.DeleteMachine(c.Context(), machineUUID.String())
	c.Check(err, tc.ErrorIs, removalerrors.MachineHasUnits)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)

	// The machine should not be gone.
	exists, err := st.MachineExists(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

func (s *machineSuite) getMachineUUIDFromApp(c *tc.C, appUUID application.ID) machine.UUID {
	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	return s.getUnitMachineUUID(c, unitUUID)
}
