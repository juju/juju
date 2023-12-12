// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice_test

import (
	"context"
	"fmt"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/blockdevice/service"
	"github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/internal/changestream/testing"
)

type watcherSuite struct {
	testing.ModelSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) createMachine(c *gc.C, name string) string {
	db := s.DB()

	netNodeUUID := utils.MustNewUUID().String()
	_, err := db.ExecContext(context.Background(),
		fmt.Sprintf("INSERT INTO net_node (uuid) VALUES ('%s')", netNodeUUID))
	c.Assert(err, jc.ErrorIsNil)
	machineUUID := utils.MustNewUUID().String()
	_, err = db.ExecContext(context.Background(),
		fmt.Sprintf(`
INSERT INTO machine (uuid, life_id, machine_id, net_node_uuid)
VALUES ('%s', %d, '%s', '%s')
`, machineUUID, 0, name, netNodeUUID))
	c.Assert(err, jc.ErrorIsNil)
	return machineUUID
}

func (s *watcherSuite) TestWatchBlockDevicesMissingMachine(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "uuid"),
		loggo.GetLogger("test"))
	service := service.NewService(st, factory, loggo.GetLogger("test"))

	_, err := service.WatchBlockDevices(context.Background(), "666")
	c.Assert(err, gc.ErrorMatches, `machine "666" not found`)
}

func (s *watcherSuite) TestStops(c *gc.C) {
	s.createMachine(c, "666")

	st := state.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "uuid"),
		loggo.GetLogger("test"))
	service := service.NewService(st, factory, loggo.GetLogger("test"))

	w, err := service.WatchBlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKill(c, w)
	c.Assert(err, gc.ErrorMatches, "getting initial block devices: context canceled")
}

func (s *watcherSuite) TestWatchBlockDevices(c *gc.C) {
	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		Size:           666,
		FilesystemType: "btrfs",
	}
	s.createMachine(c, "666")

	st := state.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "uuid"),
		loggo.GetLogger("test"))
	service := service.NewService(st, factory, loggo.GetLogger("test"))

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
		Size:           666,
		FilesystemType: "btrfs",
	}
	s.createMachine(c, "666")
	s.createMachine(c, "667")

	st := state.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "uuid"),
		loggo.GetLogger("test"))
	service := service.NewService(st, factory, loggo.GetLogger("test"))

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
