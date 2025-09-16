// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/blockdevice/service"
	"github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	testing.ModelSuite
}

func TestWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) createMachine(c *tc.C, name string) machine.UUID {
	netNodeUUID := uuid.MustNewUUID().String()
	machineUUID := tc.Must(c, machine.NewUUID)

	_, err := s.DB().Exec(`
INSERT INTO net_node (uuid) VALUES (?)
`, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO machine (uuid, life_id, name, net_node_uuid)
VALUES (?, ?, ?, ?)
`, machineUUID, 0, name, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return machineUUID
}

func (s *watcherSuite) TestStops(c *tc.C) {
	machineUUID := s.createMachine(c, "666")

	st := state.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "uuid"),
		loggertesting.WrapCheckLog(c))
	service := service.NewWatchableService(st, factory, loggertesting.WrapCheckLog(c))

	w, err := service.WatchBlockDevicesForMachine(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = workertest.CheckKill(c, w)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchBlockDevices(c *tc.C) {
	added := map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{
		"a": {
			DeviceName:     "name-666",
			SizeMiB:        666,
			FilesystemType: "btrfs",
		},
	}
	machineUUID := s.createMachine(c, "666")

	st := state.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "uuid"),
		loggertesting.WrapCheckLog(c))
	service := service.NewWatchableService(st, factory, loggertesting.WrapCheckLog(c))

	w, err := service.WatchBlockDevicesForMachine(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertKilled()

	// Initial event.
	wc.AssertOneChange()

	err = st.UpdateBlockDevicesForMachine(
		c.Context(), machineUUID, added, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Saving existing devices -> no change.
	err = st.UpdateBlockDevicesForMachine(
		c.Context(), machineUUID, nil, added, nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Updating existing device -> change.
	updated := map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{
		"a": {
			DeviceName:     "name-666",
			SizeMiB:        666,
			FilesystemType: "btrfs",
			SerialId:       "serial",
		},
	}
	err = st.UpdateBlockDevicesForMachine(
		c.Context(), machineUUID, nil, updated, nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Removing devices -> change.
	err = st.UpdateBlockDevicesForMachine(
		c.Context(), machineUUID, nil, nil, []blockdevice.BlockDeviceUUID{"a"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *watcherSuite) TestWatchBlockDevicesIgnoresWrongMachine(c *tc.C) {
	bd := map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{
		"a": {
			DeviceName:     "name-666",
			SizeMiB:        666,
			FilesystemType: "btrfs",
		},
	}
	machine1UUID := s.createMachine(c, "666")
	machine2UUID := s.createMachine(c, "667")

	st := state.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "uuid"),
		loggertesting.WrapCheckLog(c))
	service := service.NewWatchableService(st, factory, loggertesting.WrapCheckLog(c))

	w, err := service.WatchBlockDevicesForMachine(c.Context(), machine2UUID)
	c.Assert(err, tc.ErrorIsNil)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertKilled()

	// Initial event.
	wc.AssertOneChange()

	// No events for changes done to a different machine.
	err = st.UpdateBlockDevicesForMachine(c.Context(), machine1UUID, bd, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
}
