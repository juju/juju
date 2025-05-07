// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry_test

import (
	time "time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/trace"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujujujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/leaseexpiry"
)

type workerSuite struct {
	jujutesting.IsolationSuite
}

var _ = tc.Suite(&workerSuite{})

func (s *workerSuite) TestConfigValidate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	store := NewMockExpiryStore(ctrl)

	validCfg := leaseexpiry.Config{
		Clock:  clock.WallClock,
		Logger: loggertesting.WrapCheckLog(c),
		Tracer: trace.NoopTracer{},
		Store:  store,
	}

	cfg := validCfg
	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = validCfg
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = validCfg
	cfg.Store = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) TestWorker(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	clk := NewMockClock(ctrl)
	timer := NewMockTimer(ctrl)
	store := NewMockExpiryStore(ctrl)

	clk.EXPECT().NewTimer(time.Second).Return(timer)
	store.EXPECT().ExpireLeases(gomock.Any()).Return(nil)

	done := make(chan time.Duration)

	ch := make(chan time.Time, 1)
	ch <- time.Now()
	timer.EXPECT().Chan().Return(ch).MinTimes(1)
	timer.EXPECT().Reset(gomock.Any()).DoAndReturn(func(t time.Duration) bool {
		defer func() {
			select {
			case done <- t:
			case <-time.After(jujujujutesting.LongWait):
				c.Fatalf("timed out sending reset")
			}
		}()

		return true
	})
	timer.EXPECT().Stop().Return(true)

	w, err := leaseexpiry.NewWorker(leaseexpiry.Config{
		Clock:  clk,
		Logger: loggertesting.WrapCheckLog(c),
		Tracer: trace.NoopTracer{},
		Store:  store,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case t := <-done:
		// Ensure it's within the expected range.
		c.Check(t >= time.Second*1, jc.IsTrue)
		c.Check(t <= time.Second*5, jc.IsTrue)
	case <-time.After(jujujujutesting.ShortWait):
		c.Fatalf("timed out waiting for reset")
	}

	workertest.CleanKill(c, w)
}
