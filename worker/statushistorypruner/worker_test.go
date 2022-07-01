// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/core/watcher/watchertest"
	"github.com/juju/juju/v3/worker/pruner"
	"github.com/juju/juju/v3/worker/pruner/mocks"
	"github.com/juju/juju/v3/worker/statushistorypruner"
)

type PrunerSuite struct{}

var _ = gc.Suite(&PrunerSuite{})

func (s *PrunerSuite) TestRunStop(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	w := watchertest.NewMockNotifyWatcher(ch)

	facade := mocks.NewMockFacade(ctrl)
	facade.EXPECT().WatchForModelConfigChanges().Return(w, nil)

	updater, err := statushistorypruner.New(pruner.Config{
		Facade:        facade,
		PruneInterval: 0,
		Clock:         testclock.NewClock(time.Now()),
		Logger:        loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, updater)
}
