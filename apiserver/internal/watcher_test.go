// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/testing"
)

type suite struct {
	testing.BaseSuite

	watcher         *MockWatcher[[]string]
	watcherRegistry *MockWatcherRegistry
}

var _ = gc.Suite(&suite{})

func (s *suite) TestFirstResultReturnsChanges(c *gc.C) {
	defer s.setupMocks(c).Finish()

	contents := []string{"a", "b"}
	changes := make(chan []string, 1)
	changes <- contents
	s.watcher.EXPECT().Changes().Return(changes)

	res, err := internal.FirstResult[[]string](s.watcher)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.SameContents, contents)
}

func (s *suite) TestFirstResultWorkerKilled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)
	s.watcher.EXPECT().Changes().Return(changes)

	// We close the channel to make sure the worker is killed by FirstResult
	close(changes)
	s.watcher.EXPECT().Kill()
	s.watcher.EXPECT().Wait().Return(tomb.ErrDying)

	res, err := internal.FirstResult[[]string](s.watcher)
	c.Assert(err, gc.ErrorMatches, tomb.ErrDying.Error())
	c.Assert(res, gc.IsNil)
}

func (s *suite) TestFirstResultWatcherStoppedNilErr(c *gc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)
	s.watcher.EXPECT().Changes().Return(changes)

	// We close the channel to make sure the worker is killed by FirstResult
	close(changes)
	s.watcher.EXPECT().Kill()
	s.watcher.EXPECT().Wait().Return(nil)

	res, err := internal.FirstResult[[]string](s.watcher)
	c.Assert(err, gc.ErrorMatches, "expected an error from .* got nil.*")
	c.Assert(errors.Cause(err), gc.Equals, apiservererrors.ErrStoppedWatcher)
	c.Assert(res, gc.IsNil)
}

func (s *suite) TestEnsureRegisterWatcher(c *gc.C) {
	defer s.setupMocks(c).Finish()

	contents := []string{"a", "b"}
	changes := make(chan []string, 1)
	changes <- contents

	s.watcher.EXPECT().Changes().Return(changes)
	s.watcherRegistry.EXPECT().Register(s.watcher).Return("id", nil)

	id, res, err := internal.EnsureRegisterWatcher[[]string](s.watcherRegistry, s.watcher)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, gc.Equals, "id")
	c.Assert(res, jc.SameContents, contents)
}

func (s *suite) TestEnsureRegisterWatcherWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	contents := []string{"a", "b"}
	changes := make(chan []string, 1)
	changes <- contents

	s.watcher.EXPECT().Changes().Return(changes)
	s.watcherRegistry.EXPECT().Register(s.watcher).Return("id", errors.New("boom"))

	_, _, err := internal.EnsureRegisterWatcher[[]string](s.watcherRegistry, s.watcher)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *suite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcher = NewMockWatcher[[]string](ctrl)
	s.watcherRegistry = NewMockWatcherRegistry(ctrl)

	return ctrl
}
