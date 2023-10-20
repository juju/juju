// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"time"

	"github.com/juju/juju/testing"
	version "github.com/juju/version/v2"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"
)

type machineWorkerSuite struct {
	baseSuite
}

var _ = gc.Suite(&machineWorkerSuite{})

func (s *machineWorkerSuite) TestAlreadyUpgraded(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.lock.EXPECT().IsUnlocked().DoAndReturn(func() bool {
		defer close(done)
		return true
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for lock to be checked")
	}

	workertest.CleanKill(c, w)
}

func (s *machineWorkerSuite) newWorker(c *gc.C) *machineWorker {
	baseWorker := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("6.6.6"))
	return newMachineWorker(baseWorker)
}
