// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"errors"

	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/reboot"
	"github.com/juju/juju/worker/reboot/mocks"
)

type rebootSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&rebootSuite{})

func (s *rebootSuite) TestStartStop(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().WatchForRebootEvent().Return(watch, nil)
	lock := mocks.NewMockLock(ctrl)

	w, err := reboot.NewReboot(client, names.NewMachineTag("666"), lock)
	c.Assert(err, jc.ErrorIsNil)

	w.Kill()
	err = workertest.CheckKilled(c, w)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *rebootSuite) TestWorkerReboot(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().WatchForRebootEvent().Return(watch, nil)
	client.EXPECT().GetRebootAction().Return(params.ShouldReboot, nil)
	client.EXPECT().ClearReboot().Return(nil)

	lock := mocks.NewMockLock(ctrl)
	lock.EXPECT().Acquire(machinelock.Spec{
		Worker:   "reboot",
		Comment:  "reboot",
		NoCancel: true,
	}).Return(func() {}, nil)

	w, err := reboot.NewReboot(client, names.NewMachineTag("666"), lock)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Assert(err, jc.Satisfies, func(e any) bool { return errors.Is(e.(error), worker.ErrRebootMachine) })
}

func (s *rebootSuite) TestContainerShutdown(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().WatchForRebootEvent().Return(watch, nil)
	client.EXPECT().GetRebootAction().Return(params.ShouldShutdown, nil)
	client.EXPECT().ClearReboot().Return(nil)

	lock := mocks.NewMockLock(ctrl)
	lock.EXPECT().Acquire(machinelock.Spec{
		Worker:   "reboot",
		Comment:  "shutdown",
		NoCancel: true,
	}).Return(func() {}, nil)

	w, err := reboot.NewReboot(client, names.NewMachineTag("666/lxd/0"), lock)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Assert(err, jc.Satisfies, func(e any) bool { return errors.Is(e.(error), worker.ErrShutdownMachine) })
}
