// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry_test

import (
	time "time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database/testing"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/leaseexpiry"
)

type workerSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	store := NewMockExpiryStore(ctrl)

	validCfg := leaseexpiry.Config{
		Clock:  clock.WallClock,
		Logger: jujutesting.CheckLogger{Log: c},
		Store:  store,
	}

	cfg := validCfg
	cfg.Clock = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = validCfg
	cfg.Logger = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = validCfg
	cfg.Store = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)
}

func (s *workerSuite) TestWorker(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	clk := NewMockClock(ctrl)
	timer := NewMockTimer(ctrl)
	store := NewMockExpiryStore(ctrl)

	clk.EXPECT().NewTimer(time.Second).Return(timer)
	store.EXPECT().ExpireLeases(gomock.Any()).Return(nil)

	done := make(chan struct{})

	ch := make(chan time.Time, 1)
	ch <- time.Now()
	timer.EXPECT().Chan().Return(ch).MinTimes(1)
	timer.EXPECT().Reset(time.Second).Do(func(any) {
		defer close(done)
	})
	timer.EXPECT().Stop().Return(true)

	w, err := leaseexpiry.NewWorker(leaseexpiry.Config{
		Clock:  clk,
		Logger: jujutesting.CheckLogger{Log: c},
		Store:  store,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(jujutesting.ShortWait):
		c.Fatalf("timed out waiting for reset")
	}

	workertest.CleanKill(c, w)
}
