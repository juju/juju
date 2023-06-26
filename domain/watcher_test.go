// Copyright 2033 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"errors"
	"time"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/database/testing"
	jujutesting "github.com/juju/juju/testing"
)

type watcherSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&watcherSuite{})

func (*watcherSuite) TestNewUUIDsWatcherFail(c *gc.C) {
	factory := NewWatcherFactory(func() (changestream.WatchableDB, error) {
		return nil, errors.New("fail getting db instance")
	}, nil)

	_, err := factory.NewUUIDsWatcher(changestream.All, "random_namespace")
	c.Assert(err, gc.ErrorMatches, "creating base watcher: fail getting db instance")
}

func (s *watcherSuite) TestNewUUIDsWatcherSuccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	changes := make(chan []changestream.ChangeEvent)
	done := make(chan struct{})

	// These expectations are very soft.
	// We are only testing that the factory produces a functioning worker.
	// The workers themselves are properly tested at their package sites.
	sub := NewMockSubscription(ctrl)
	sub.EXPECT().Changes().Return(changes)
	sub.EXPECT().Unsubscribe()
	sub.EXPECT().Done().Return(done).AnyTimes()

	events := NewMockEventSource(ctrl)
	events.EXPECT().Subscribe(gomock.Any()).Return(sub, nil)

	factory := NewWatcherFactory(func() (changestream.WatchableDB, error) {
		return &watchableDB{
			TxnRunner:   s.TxnRunner(),
			EventSource: events,
		}, nil
	}, nil)

	w, err := factory.NewUUIDsWatcher(changestream.All, "external_controller")
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-w.Changes():
	case <-time.After(jujutesting.ShortWait):
		c.Fatal("timed out waiting for change event")
	}

	workertest.CleanKill(c, w)
}

type watchableDB struct {
	database.TxnRunner
	changestream.EventSource
}
