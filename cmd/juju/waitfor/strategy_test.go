// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package waitfor

import (
	"context"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/waitfor/api"
	"github.com/juju/juju/cmd/juju/waitfor/api/mocks"
	"github.com/juju/juju/cmd/juju/waitfor/query"
	"github.com/juju/juju/rpc/params"
)

type strategySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&strategySuite{})

func (s *strategySuite) TestRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expected := []params.Delta{{
		Entity: &MockEntityInfo{
			Name: "meshuggah",
		},
	}}

	allWatcher := mocks.NewMockAllWatcher(ctrl)
	allWatcher.EXPECT().Next().Return(expected, nil)
	allWatcher.EXPECT().Stop()

	client := mocks.NewMockWatchAllAPI(ctrl)
	client.EXPECT().WatchAll().Return(allWatcher, nil)

	var executed bool
	var deltas []params.Delta

	strategy := Strategy{
		ClientFn: func() (api.WatchAllAPI, error) {
			return client, nil
		},
		Timeout: time.Minute,
	}
	err := strategy.Run(context.Background(), "generic", `life=="active"`, func(_ string, d []params.Delta, _ query.Query) (bool, error) {
		executed = true
		deltas = d
		return true, nil
	}, emptyNotify)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(executed, jc.IsTrue)
	c.Assert(deltas, gc.DeepEquals, expected)
}

func (s *strategySuite) TestRunWithCallback(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expected := []params.Delta{{
		Entity: &MockEntityInfo{
			Name: "meshuggah",
		},
	}}

	allWatcher := mocks.NewMockAllWatcher(ctrl)
	allWatcher.EXPECT().Next().Return(expected, nil)
	allWatcher.EXPECT().Stop()

	client := mocks.NewMockWatchAllAPI(ctrl)
	client.EXPECT().WatchAll().Return(allWatcher, nil)

	var eventType EventType

	strategy := Strategy{
		ClientFn: func() (api.WatchAllAPI, error) {
			return client, nil
		},
		Timeout: time.Minute,
	}
	strategy.Subscribe(func(event EventType) {
		eventType = event
	})
	err := strategy.Run(context.Background(), "generic", `life=="active"`, func(_ string, d []params.Delta, _ query.Query) (bool, error) {
		return true, nil
	}, emptyNotify)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eventType, gc.Equals, WatchAllStarted)
}

func (s *strategySuite) TestRunWithInvalidQuery(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockWatchAllAPI(ctrl)
	strategy := Strategy{
		ClientFn: func() (api.WatchAllAPI, error) {
			return client, nil
		},
		Timeout: time.Minute,
	}
	err := strategy.Run(context.Background(), "generic", `life=="ac`, func(_ string, d []params.Delta, _ query.Query) (bool, error) {
		c.FailNow()
		return false, nil
	}, emptyNotify)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `
Cannot parse query: string is not correctly terminated.

1 | life=="ac
          ^
Try adding a closing quote to the string.`[1:])
}

type MockEntityInfo struct {
	Name    string `json:"name"`
	Integer int    `json:"int"`
	Boolean bool   `json:"bool"`
}

func (m *MockEntityInfo) EntityId() params.EntityId {
	return params.EntityId{}
}
