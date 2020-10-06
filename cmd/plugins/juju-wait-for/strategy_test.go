// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/api/mocks"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/query"
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
		Client:  client,
		Timeout: time.Minute,
	}
	err := strategy.Run("generic", `life=="active"`, func(_ string, d []params.Delta, _ query.Query) bool {
		executed = true
		deltas = d
		return true
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(executed, jc.IsTrue)
	c.Assert(deltas, gc.DeepEquals, expected)
}

func (s *strategySuite) TestRunWithInvalidQuery(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockWatchAllAPI(ctrl)
	strategy := Strategy{
		Client:  client,
		Timeout: time.Minute,
	}
	err := strategy.Run("generic", `life=="ac`, func(_ string, d []params.Delta, _ query.Query) bool {
		c.FailNow()
		return false
	})
	c.Assert(err, gc.ErrorMatches, `Syntax Error:<:1:7> invalid character '<UNKNOWN>' found`)
}

type genericScopeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&genericScopeSuite{})

func (s *genericScopeSuite) TestGetIdentValue(c *gc.C) {
	tests := []struct {
		Field    string
		Info     *MockEntityInfo
		Expected query.Ord
	}{{
		Field:    "name",
		Info:     &MockEntityInfo{Name: "generic name"},
		Expected: query.NewString("generic name"),
	}, {
		Field:    "int",
		Info:     &MockEntityInfo{Integer: 1},
		Expected: query.NewInteger(int64(1)),
	}, {
		Field:    "bool",
		Info:     &MockEntityInfo{Boolean: true},
		Expected: query.NewBool(true),
	}}
	for i, test := range tests {
		c.Logf("%d: GetIdentValue %q", i, test.Field)
		scope := GenericScope{
			Info: test.Info,
		}
		result, err := scope.GetIdentValue(test.Field)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, gc.DeepEquals, test.Expected)
	}
}

func (s *genericScopeSuite) TestGetIdentValueError(c *gc.C) {
	scope := GenericScope{
		Info: &MockEntityInfo{},
	}
	result, err := scope.GetIdentValue("bad")
	c.Assert(err, gc.ErrorMatches, `Runtime Error: identifier "bad" not found on Info`)
	c.Assert(result, gc.IsNil)
}

type MockEntityInfo struct {
	Name    string `json:"name"`
	Integer int    `json:"int"`
	Boolean bool   `json:"bool"`
}

func (m *MockEntityInfo) EntityId() params.EntityId {
	return params.EntityId{}
}
