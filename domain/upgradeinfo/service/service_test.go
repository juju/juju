// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	uistate "github.com/juju/juju/domain/upgradeinfo/state"
)

type serviceSuite struct {
	testing.IsolationSuite
	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func mustParseTime(t string) time.Time {
	ttime, err := time.Parse(time.RFC3339, t)
	if err != nil {
		panic("cannot parse time")
	}
	return ttime
}

func (s *serviceSuite) TestEnsureUpgradeInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().EnsureUpgradeInfo(gomock.Any(), "1", version.MustParse("3.0.0"), version.MustParse("3.0.1")).Return(uistate.Info{
		PreviousVersion: "3.0.0",
		TargetVersion:   "3.0.1",
		InitTime:        "2023-06-20T15:37:17Z",
		StartTime:       "2023-06-20T15:37:18Z",
	}, []uistate.InfoControllerNode{{
		ControllerNodeID: "0",
		NodeStatus:       "done",
	}, {
		ControllerNodeID: "1",
		NodeStatus:       "ready",
	}}, nil)

	info, err := NewService(s.state).EnsureUpgradeInfo(context.Background(), "1", version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.DeepEquals, Info{
		PreviousVersion:  version.MustParse("3.0.0"),
		TargetVersion:    version.MustParse("3.0.1"),
		InitTime:         mustParseTime("2023-06-20T15:37:17Z"),
		StartTime:        mustParseTime("2023-06-20T15:37:18Z"),
		ControllersReady: []string{"1"},
		ControllersDone:  []string{"0"},
	})
}

func (s *serviceSuite) TestIsUpgradingTrue(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsUpgrading(gomock.Any()).Return(true, nil)

	upgrading, err := NewService(s.state).IsUpgrading(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(upgrading, jc.IsTrue)
}
