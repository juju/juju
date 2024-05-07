// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/worker/pruner"
	"github.com/juju/juju/internal/worker/pruner/mocks"
	"github.com/juju/juju/internal/worker/statushistorypruner"
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
		"max-status-history-size": "0",
		"max-status-history-age":  "0",
	})
	modelConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	facade := mocks.NewMockFacade(ctrl)

	service := mocks.NewMockModelConfigService(ctrl)
	service.EXPECT().Watch().Return(w, nil)
	service.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil).AnyTimes()

	updater, err := statushistorypruner.New(pruner.Config{
		Facade:             facade,
		ModelConfigService: service,
		PruneInterval:      0,
		Clock:              testclock.NewClock(time.Now()),
		Logger:             loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, updater)
}
