// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"time"

	jc "github.com/juju/testing/checkers"
	version "github.com/juju/version/v2"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type controllerWorkerSuite struct {
	baseSuite

	upgradeService *MockUpgradeService
}

var _ = gc.Suite(&controllerWorkerSuite{})

func (s *controllerWorkerSuite) TestAlreadyUpgraded(c *gc.C) {
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

func (s *controllerWorkerSuite) newWorker(c *gc.C) *controllerWorker {
	baseWorker := s.newBaseWorker(c, version.MustParse("6.6.6"), version.MustParse("6.6.6"))
	w, err := newControllerWorker(baseWorker, s.upgradeService)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *controllerWorkerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.upgradeService = NewMockUpgradeService(ctrl)

	return ctrl
}
