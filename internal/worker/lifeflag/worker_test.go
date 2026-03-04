// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	apilifeflag "github.com/juju/juju/api/agent/lifeflag"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/lifeflag"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite
}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &WorkerSuite{})
}

func (*WorkerSuite) TestCreateNotFoundError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := NewMockFacade(ctrl)
	facade.EXPECT().Life(
		gomock.Any(), tag,
	).Return("", apilifeflag.ErrEntityNotFound)

	config := lifeflag.Config{
		Facade: facade,
		Entity: tag,
		Result: explode,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorIs, apilifeflag.ErrEntityNotFound)
}

func (*WorkerSuite) TestCreateRandomError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := NewMockFacade(ctrl)
	facade.EXPECT().Life(gomock.Any(), tag).Return("", errors.New("boom splat"))

	config := lifeflag.Config{
		Facade: facade,
		Entity: tag,
		Result: explode,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "boom splat")
}

func (*WorkerSuite) TestWatchNotFoundError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := NewMockFacade(ctrl)
	facade.EXPECT().Life(gomock.Any(), tag).Return(life.Alive, nil)
	facade.EXPECT().Watch(
		gomock.Any(), tag,
	).Return(nil, apilifeflag.ErrEntityNotFound)

	config := lifeflag.Config{
		Facade: facade,
		Entity: tag,
		Result: never,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorIs, apilifeflag.ErrEntityNotFound)
}

func (*WorkerSuite) TestWatchRandomError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := NewMockFacade(ctrl)
	facade.EXPECT().Life(gomock.Any(), tag).Return(life.Alive, nil)
	facade.EXPECT().Watch(gomock.Any(), tag).Return(nil, errors.New("pew pew"))

	config := lifeflag.Config{
		Facade: facade,
		Entity: tag,
		Result: never,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorMatches, "pew pew")
}

func (*WorkerSuite) TestLifeNotFoundError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Pre-fill one event; since Result is never (always false) there is no
	// observable race on the initial Check() call even if the goroutine
	// processes the event first.
	ch := make(chan struct{}, 1)
	ch <- struct{}{}

	facade := NewMockFacade(ctrl)
	facade.EXPECT().Life(gomock.Any(), tag).Return(life.Alive, nil)
	facade.EXPECT().Watch(
		gomock.Any(), tag,
	).Return(watchertest.NewMockNotifyWatcher(ch), nil)
	facade.EXPECT().Life(
		gomock.Any(), tag,
	).Return("", apilifeflag.ErrEntityNotFound)

	config := lifeflag.Config{
		Facade: facade,
		Entity: tag,
		Result: never,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorIs, apilifeflag.ErrEntityNotFound)
}

func (*WorkerSuite) TestLifeRandomError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Pre-fill one event; since Result is never (always false) there is no
	// observable race on the initial Check() call even if the goroutine
	// processes the event first.
	ch := make(chan struct{}, 1)
	ch <- struct{}{}

	facade := NewMockFacade(ctrl)
	facade.EXPECT().Life(gomock.Any(), tag).Return(life.Alive, nil)
	facade.EXPECT().Watch(
		gomock.Any(), tag,
	).Return(watchertest.NewMockNotifyWatcher(ch), nil)
	facade.EXPECT().Life(
		gomock.Any(), tag,
	).Return("", errors.New("rawr"))

	config := lifeflag.Config{
		Facade: facade,
		Entity: tag,
		Result: never,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorMatches, "rawr")
}

func (*WorkerSuite) TestResultImmediateRealChange(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	done := make(chan struct{})
	ch := make(chan struct{}, 1)

	facade := NewMockFacade(ctrl)
	facade.EXPECT().Life(gomock.Any(), tag).Return(life.Alive, nil)
	facade.EXPECT().Watch(
		gomock.Any(), tag,
	).Return(watchertest.NewMockNotifyWatcher(ch), nil)
	facade.EXPECT().Life(
		gomock.Any(), tag,
	).DoAndReturn(func(context.Context, names.Tag) (life.Value, error) {
		close(done)
		return life.Dead, nil
	})

	config := lifeflag.Config{
		Facade: facade,
		Entity: tag,
		Result: life.IsNotAlive,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(worker.Check(), tc.IsFalse)

	ch <- struct{}{}

	select {
	case <-done:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for worker to change state")
	}

	c.Assert(worker.Check(), tc.IsTrue)

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, tc.Equals, lifeflag.ErrValueChanged)
}

func (*WorkerSuite) TestResultSubsequentRealChange(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	done := make(chan struct{})
	ch := make(chan struct{}, 2)

	facade := NewMockFacade(ctrl)
	facade.EXPECT().Life(gomock.Any(), tag).Return(life.Dying, nil)
	facade.EXPECT().Watch(
		gomock.Any(), tag,
	).Return(watchertest.NewMockNotifyWatcher(ch), nil)
	facade.EXPECT().Life(gomock.Any(), tag).Return(life.Dying, nil)
	facade.EXPECT().Life(
		gomock.Any(), tag,
	).DoAndReturn(func(context.Context, names.Tag) (life.Value, error) {
		close(done)
		return life.Dead, nil
	})

	config := lifeflag.Config{
		Facade: facade,
		Entity: tag,
		Result: life.IsNotDead,
	}
	worker, err := lifeflag.New(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(worker.Check(), tc.IsTrue)

	ch <- struct{}{}
	ch <- struct{}{}
	select {
	case <-done:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for worker to change state")
	}

	c.Assert(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, tc.Equals, lifeflag.ErrValueChanged)
}

func (*WorkerSuite) TestResultNoRealChange(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 2)
	ch <- struct{}{}
	ch <- struct{}{}

	facade := NewMockFacade(ctrl)
	facade.EXPECT().Life(gomock.Any(), tag).Return(life.Alive, nil)
	facade.EXPECT().Watch(
		gomock.Any(), tag,
	).Return(watchertest.NewMockNotifyWatcher(ch), nil)
	facade.EXPECT().Life(gomock.Any(), tag).Return(life.Alive, nil)
	facade.EXPECT().Life(gomock.Any(), tag).Return(life.Dying, nil)

	config := lifeflag.Config{
		Facade: facade,
		Entity: tag,
		Result: life.IsNotDead,
	}
	worker, err := lifeflag.New(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(worker.Check(), tc.IsTrue)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
}
