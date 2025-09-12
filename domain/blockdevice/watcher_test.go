// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice_test

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/blockdevice/service"
	"github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/errors"
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
	db := s.TxnRunner()

	netNodeUUID := uuid.MustNewUUID().String()
	machineUUID := tc.Must(c, machine.NewUUID)

	queryNetNode := `
INSERT INTO net_node (uuid) VALUES (?)
`
	queryMachine := `
INSERT INTO machine (uuid, life_id, name, net_node_uuid)
VALUES (?, ?, ?, ?)
	`

	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, queryNetNode, netNodeUUID); err != nil {
			return errors.Capture(err)
		}

		if _, err := tx.ExecContext(ctx, queryMachine, machineUUID, 0, name, netNodeUUID); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
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

	w, err := service.WatchBlockDevices(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = workertest.CheckKill(c, w)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchBlockDevices(c *tc.C) {
	added := map[string]blockdevice.BlockDevice{
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

	w, err := service.WatchBlockDevices(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertKilled()

	// Initial event.
	wc.AssertOneChange()

	err = st.UpdateMachineBlockDevices(
		c.Context(), machineUUID, added, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Saving existing devices -> no change.
	err = st.UpdateMachineBlockDevices(
		c.Context(), machineUUID, nil, added, nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Updating existing device -> change.
	updated := map[string]blockdevice.BlockDevice{
		"a": {
			DeviceName:     "name-666",
			SizeMiB:        666,
			FilesystemType: "btrfs",
			SerialId:       "serial",
		},
	}
	err = st.UpdateMachineBlockDevices(
		c.Context(), machineUUID, nil, updated, nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Removing devices -> change.
	err = st.UpdateMachineBlockDevices(
		c.Context(), machineUUID, nil, nil, []string{"a"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *watcherSuite) TestWatchBlockDevicesIgnoresWrongMachine(c *tc.C) {
	bd := map[string]blockdevice.BlockDevice{
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

	w, err := service.WatchBlockDevices(c.Context(), machine2UUID)
	c.Assert(err, tc.ErrorIsNil)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertKilled()

	// Initial event.
	wc.AssertOneChange()

	// No events for changes done to a different machine.
	err = st.UpdateMachineBlockDevices(c.Context(), machine1UUID, bd, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
}
