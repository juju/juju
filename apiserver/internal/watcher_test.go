// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/testing"
)

type suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&suite{})

func (suite) TestFirstResultReturnsChanges(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	stringsWatcher := NewMockWatcher[[]string](ctrl)
	contents := []string{"a", "b"}
	changes := make(chan []string, 1)
	changes <- contents
	stringsWatcher.EXPECT().Changes().Return(changes)

	res, err := internal.FirstResult[[]string](stringsWatcher)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.SameContents, contents)
}

func (suite) TestFirstResultWorkerKilled(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	stringsWatcher := NewMockWatcher[[]string](ctrl)
	changes := make(chan []string, 1)
	stringsWatcher.EXPECT().Changes().Return(changes)

	// We close the channel to make sure the worker is killed by FirstResult
	close(changes)
	stringsWatcher.EXPECT().Kill()
	stringsWatcher.EXPECT().Wait().Return(tomb.ErrDying)

	res, err := internal.FirstResult[[]string](stringsWatcher)
	c.Assert(err, gc.ErrorMatches, tomb.ErrDying.Error())
	c.Assert(res, gc.IsNil)
}

func (suite) TestFirstResultWatcherStoppedNilErr(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	stringsWatcher := NewMockWatcher[[]string](ctrl)
	changes := make(chan []string, 1)
	stringsWatcher.EXPECT().Changes().Return(changes)

	// We close the channel to make sure the worker is killed by FirstResult
	close(changes)
	stringsWatcher.EXPECT().Kill()
	stringsWatcher.EXPECT().Wait().Return(nil)

	res, err := internal.FirstResult[[]string](stringsWatcher)
	c.Assert(err, gc.ErrorMatches, "expected an error from .* got nil.*")
	c.Assert(errors.Cause(err), gc.Equals, apiservererrors.ErrStoppedWatcher)
	c.Assert(res, gc.IsNil)
}
