// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actionpruner_test

import (
	"time"

	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
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

	ch := make(chan []string, 1)
	ch <- []string{}
	w := watchertest.NewMockStringsWatcher(ch)

	attrs := coretesting.FakeConfig().Merge(map[string]interface{}{
		"max-action-results-age":  "2h",
		"max-action-results-size": "2GiB",
	})
	modelConfig, err := config.New(false, attrs)
	c.Assert(err, jc.ErrorIsNil)

	facade := mocks.NewMockFacade(ctrl)

	service := mocks.NewMockModelConfigService(ctrl)
	service.EXPECT().Watch().Return(w, nil)
	service.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil).AnyTimes()

	updater, err := actionpruner.New(pruner.Config{
		Facade:             facade,
		ModelConfigService: service,
		PruneInterval:      time.Minute,
		Clock:              testclock.NewClock(time.Now()),
		Logger:             loggertesting.WrapCheckLog(c),
	})

	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, updater)
}
