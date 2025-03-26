// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/errors"
)

type leadershipSuite struct {
	testing.IsolationSuite

	state *MockLeadershipState
}

var _ = gc.Suite(&leadershipSuite{})

func (s *leadershipSuite) TestLeadershipService(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationLeadership(gomock.Any()).Return(map[string]string{
		"foo": "bar",
	}, nil)

	leadershipService := NewLeadershipService(s.state)
	leaders, err := leadershipService.GetApplicationLeadership(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(leaders, jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *leadershipSuite) TestLeadershipServiceError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationLeadership(gomock.Any()).Return(map[string]string{
		"foo": "bar",
	}, errors.Errorf("boom"))

	leadershipService := NewLeadershipService(s.state)
	_, err := leadershipService.GetApplicationLeadership(context.Background())
	c.Assert(err, gc.ErrorMatches, "boom")

}

func (s *leadershipSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockLeadershipState(ctrl)

	return ctrl
}
