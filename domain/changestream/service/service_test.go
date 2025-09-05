// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/juju/domain/changestream"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"
)

type serviceSuite struct {
	state *MockState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestPrune(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	s.state.EXPECT().Prune(gomock.Any(), gomock.Any()).Return(changestream.Window{
		Start: now,
		End:   now.Add(time.Hour),
	}, int64(42), nil)

	svc := &Service{
		st: s.state,
	}

	result, n, err := svc.Prune(c.Context(), changestream.Window{})
	c.Assert(err, tc.IsNil)
	c.Check(n, tc.Equals, int64(42))
	c.Check(result, tc.DeepEquals, changestream.Window{
		Start: now,
		End:   now.Add(time.Hour),
	})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	return ctrl
}
