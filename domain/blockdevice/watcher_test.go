// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice_test

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/changestream"
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

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) createMachine(c *gc.C, name string) string {
	db := s.TxnRunner()

	netNodeUUID := uuid.MustNewUUID().String()
	machineUUID := uuid.MustNewUUID().String()

	queryNetNode := `
INSERT INTO net_node (uuid) VALUES (?)
`
	queryMachine := `
INSERT INTO machine (uuid, life_id, name, net_node_uuid)
VALUES (?, ?, ?, ?)
	`

	err := db.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, queryNetNode, netNodeUUID); err != nil {
			return errors.Capture(err)
		}

		if _, err := tx.ExecContext(ctx, queryMachine, machineUUID, 0, name, netNodeUUID); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	return machineUUID
}

func (s *watcherSuite) TestWatchBlockDevicesMissingMachine(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "uuid"),
		loggertesting.WrapCheckLog(c))
	service := service.NewWatchableService(st, factory, loggertesting.WrapCheckLog(c))

	_, err := service.WatchBlockDevices(context.Background(), "666")
	c.Assert(err, gc.ErrorMatches, `machine "666" not found`)
}

func (s *watcherSuite) TestStops(c *gc.C) {
	s.createMachine(c, "666")

	st := state.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "uuid"),
		loggertesting.WrapCheckLog(c))
	service := service.NewWatchableService(st, factory, loggertesting.WrapCheckLog(c))

	w, err := service.WatchBlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKill(c, w)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchBlockDevices(c *gc.C) {
	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
	}
	s.createMachine(c, "666")

	st := state.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "uuid"),
		loggertesting.WrapCheckLog(c))
	service := service.NewWatchableService(st, factory, loggertesting.WrapCheckLog(c))

	w, err := service.WatchBlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertKilled()

	// Initial event.
	wc.AssertOneChange()

	err = st.SetMachineBlockDevices(context.Background(), "666", bd)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Saving existing devices -> no change.
	err = st.SetMachineBlockDevices(context.Background(), "666", bd)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Updating existing device -> change.
	bd.SerialId = "serial"
	err = st.SetMachineBlockDevices(context.Background(), "666", bd)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Removing devices -> change.
	err = st.SetMachineBlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *watcherSuite) TestWatchBlockDevicesIgnoresWrongMachine(c *gc.C) {
	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
	}
	s.createMachine(c, "666")
	s.createMachine(c, "667")

	st := state.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "uuid"),
		loggertesting.WrapCheckLog(c))
	service := service.NewWatchableService(st, factory, loggertesting.WrapCheckLog(c))

	w, err := service.WatchBlockDevices(context.Background(), "667")
	c.Assert(err, jc.ErrorIsNil)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertKilled()

	// Initial event.
	wc.AssertOneChange()

	// No events for changes done to a different machine.
	err = st.SetMachineBlockDevices(context.Background(), "666", bd)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}
