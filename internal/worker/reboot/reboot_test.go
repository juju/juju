// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/reboot"
	"github.com/juju/juju/internal/worker/reboot/mocks"
	"github.com/juju/juju/rpc/params"
)

type rebootSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&rebootSuite{})

func (s *rebootSuite) TestStartStop(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().WatchForRebootEvent(gomock.Any()).Return(watch, nil)
	lock := mocks.NewMockLock(ctrl)

	w, err := reboot.NewReboot(client, names.NewMachineTag("666"), lock)
	c.Assert(err, tc.ErrorIsNil)

	w.Kill()
	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *rebootSuite) TestWorkerReboot(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().WatchForRebootEvent(gomock.Any()).Return(watch, nil)
	client.EXPECT().GetRebootAction(gomock.Any()).Return(params.ShouldReboot, nil)
	client.EXPECT().ClearReboot(gomock.Any()).Return(nil)

	lock := mocks.NewMockLock(ctrl)
	lock.EXPECT().Acquire(machinelock.Spec{
		Worker:   "reboot",
		Comment:  "reboot",
		NoCancel: true,
	}).Return(func() {}, nil)

	w, err := reboot.NewReboot(client, names.NewMachineTag("666"), lock)
	c.Assert(err, tc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, worker.ErrRebootMachine)
}

func (s *rebootSuite) TestContainerShutdown(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().WatchForRebootEvent(gomock.Any()).Return(watch, nil)
	client.EXPECT().GetRebootAction(gomock.Any()).Return(params.ShouldShutdown, nil)
	client.EXPECT().ClearReboot(gomock.Any()).Return(nil)

	lock := mocks.NewMockLock(ctrl)
	lock.EXPECT().Acquire(machinelock.Spec{
		Worker:   "reboot",
		Comment:  "shutdown",
		NoCancel: true,
	}).Return(func() {}, nil)

	w, err := reboot.NewReboot(client, names.NewMachineTag("666/lxd/0"), lock)
	c.Assert(err, tc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, worker.ErrShutdownMachine)
}
