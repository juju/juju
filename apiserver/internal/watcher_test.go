// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/internal/testing"
)

type suite struct {
	testing.BaseSuite

	watcher         *MockWatcher[[]string]
	watcherRegistry *MockWatcherRegistry
}

func TestSuite(t *stdtesting.T) { tc.Run(t, &suite{}) }
func (s *suite) TestFirstResultReturnsChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	contents := []string{"a", "b"}
	changes := make(chan []string, 1)
	changes <- contents
	s.watcher.EXPECT().Changes().Return(changes)

	res, err := internal.FirstResult[[]string](c.Context(), s.watcher)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.SameContents, contents)
}

func (s *suite) TestFirstResultWorkerKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)
	s.watcher.EXPECT().Changes().Return(changes)

	// We close the channel to make sure the worker is killed by FirstResult
	close(changes)
	s.watcher.EXPECT().Kill()
	s.watcher.EXPECT().Wait().Return(tomb.ErrDying)

	res, err := internal.FirstResult[[]string](c.Context(), s.watcher)
	c.Assert(err, tc.ErrorMatches, tomb.ErrDying.Error())
	c.Assert(res, tc.IsNil)
}

func (s *suite) TestFirstResultWatcherStoppedNilErr(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)
	s.watcher.EXPECT().Changes().Return(changes)

	// We close the channel to make sure the worker is killed by FirstResult
	close(changes)
	s.watcher.EXPECT().Kill()
	s.watcher.EXPECT().Wait().Return(nil)

	res, err := internal.FirstResult[[]string](c.Context(), s.watcher)
	c.Assert(err, tc.ErrorMatches, "expected an error from .* got nil.*")
	c.Assert(errors.Cause(err), tc.Equals, apiservererrors.ErrStoppedWatcher)
	c.Assert(res, tc.IsNil)
}

func (s *suite) TestEnsureRegisterWatcher(c *tc.C) {
	defer s.setupMocks(c).Finish()

	contents := []string{"a", "b"}
	changes := make(chan []string, 1)
	changes <- contents

	s.watcher.EXPECT().Changes().Return(changes)
	s.watcherRegistry.EXPECT().Register(s.watcher).Return("id", nil)

	id, res, err := internal.EnsureRegisterWatcher[[]string](c.Context(), s.watcherRegistry, s.watcher)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(id, tc.Equals, "id")
	c.Assert(res, tc.SameContents, contents)
}

func (s *suite) TestEnsureRegisterWatcherWithError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	contents := []string{"a", "b"}
	changes := make(chan []string, 1)
	changes <- contents

	s.watcher.EXPECT().Changes().Return(changes)
	s.watcherRegistry.EXPECT().Register(s.watcher).Return("id", errors.New("boom"))

	_, _, err := internal.EnsureRegisterWatcher[[]string](c.Context(), s.watcherRegistry, s.watcher)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *suite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcher = NewMockWatcher[[]string](ctrl)
	s.watcherRegistry = NewMockWatcherRegistry(ctrl)

	return ctrl
}
