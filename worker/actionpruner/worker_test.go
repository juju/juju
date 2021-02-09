// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actionpruner_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/actionpruner"
	"github.com/juju/juju/worker/pruner"
	"github.com/juju/juju/worker/pruner/mocks"
)

type PrunerSuite struct{}

var _ = gc.Suite(&PrunerSuite{})

func (s *PrunerSuite) TestRunStop(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := mocks.NewMockFacade(ctrl)
	facade.EXPECT().WatchForModelConfigChanges().Return(nil, errors.NotSupportedf(""))
	facade.EXPECT().WatchForControllerConfigChanges().Return(nil, errors.NotSupportedf(""))

	updater, err := actionpruner.New(pruner.Config{
		Facade:        facade,
		PruneInterval: 0,
		Clock:         testclock.NewClock(time.Now()),
		Logger:        loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, updater)
}
