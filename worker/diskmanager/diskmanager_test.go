// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/diskmanager"
)

var _ = gc.Suite(&DiskManagerWorkerSuite{})

type DiskManagerWorkerSuite struct {
	coretesting.BaseSuite
}

func (s *DiskManagerWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(diskmanager.BlockDeviceInUse, func(storage.BlockDevice) (bool, error) {
		return false, nil
	})
}

func (s *DiskManagerWorkerSuite) TestWorkerNoOp(c *gc.C) {
	s.PatchValue(&version.Current.OS, version.Windows)

	// On Windows, the worker is a no-op worker.
	var called bool
	s.PatchValue(diskmanager.NewNoOpWorker, func() worker.Worker {
		called = true
		return worker.NewNoOpWorker()
	})

	diskmanager.NewWorker(nil)
	c.Assert(called, jc.IsTrue)
}

type BlockDeviceSetterFunc func([]storage.BlockDevice) error

func (f BlockDeviceSetterFunc) SetMachineBlockDevices(devices []storage.BlockDevice) error {
	return f(devices)
}
