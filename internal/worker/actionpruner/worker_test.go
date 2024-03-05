// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actionpruner_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/worker/actionpruner"
	"github.com/juju/juju/internal/worker/pruner"
	"github.com/juju/juju/internal/worker/pruner/mocks"
	coretesting "github.com/juju/juju/testing"
)

type PrunerSuite struct{}

var _ = gc.Suite(&PrunerSuite{})

func (s *PrunerSuite) TestRunStop(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, err := config.New(false, map[string]interface{}{
		"name":                    "test",
		"type":                    "manual",
		"uuid":                    coretesting.ModelTag.Id(),
		"max-action-results-age":  "2h",
		"max-action-results-size": "2GiB",
	})
	c.Assert(err, jc.ErrorIsNil)

	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	w := watchertest.NewMockNotifyWatcher(ch)

	facade := mocks.NewMockFacade(ctrl)
	facade.EXPECT().WatchForModelConfigChanges().Return(w, nil)

	// Depending on the host compute speed, the loop may select either
	// the watcher change event, or the catacomb's dying event first.
	facade.EXPECT().ModelConfig(context.Background()).Return(cfg, nil).AnyTimes()

	updater, err := actionpruner.New(pruner.Config{
		Facade:        facade,
		PruneInterval: time.Minute,
		Clock:         testclock.NewClock(time.Now()),
		Logger:        loggo.GetLogger("test"),
	})

	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, updater)
}
