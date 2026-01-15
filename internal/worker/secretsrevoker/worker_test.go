// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/worker/secretsrevoker"
)

type workerSuite struct {
	testing.LoggingSuite

	facade *MockSecretsFacade
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.facade = NewMockSecretsFacade(ctrl)
	return ctrl
}

func (s *workerSuite) TestWorker(c *gc.C) {
	defer s.setupMocks(c).Finish()

	clk := testclock.NewDilatedWallClock(time.Millisecond)
	now := clk.Now()
	last := now.Add(10 * time.Minute)

	ch := make(chan []string, 1)
	ch <- []string(nil)
	expiryWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.CheckKilled(c, expiryWatcher)
	s.facade.EXPECT().WatchIssuedTokenExpiry().Return(expiryWatcher, nil)

	done := make(chan struct{})
	s.facade.EXPECT().RevokeIssuedTokens(gomock.Any()).DoAndReturn(func(until time.Time) error {
		close(done)
		c.Assert(until, jc.After, last)
		return nil
	})

	w, err := secretsrevoker.NewWorker(secretsrevoker.Config{
		Facade: s.facade,
		Logger: loggo.GetLogger("test"),
		Clock:  clk,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	ch <- []string{last.Format(time.RFC3339)}
	<-done
}
