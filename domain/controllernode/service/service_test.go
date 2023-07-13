// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestUpdateExternalControllerSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CurateNodes(gomock.Any(), []string{"3", "4"}, []string{"1"})

	err := NewService(s.state).CurateNodes(context.Background(), []string{"3", "4"}, []string{"1"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateDqliteNode(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().UpdateDqliteNode(gomock.Any(), "0", "whatever", "192.168.5.60")

	err := NewService(s.state).UpdateDqliteNode(context.Background(), "0", "whatever", "192.168.5.60")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestIsModelKnownToController(c *gc.C) {
	defer s.setupMocks(c).Finish()

	knownID := "is-a-known-model"

	exp := s.state.EXPECT()
	gomock.InOrder(
		exp.SelectModelUUID(gomock.Any(), "is-not-there").Return("", errors.NotFound),
		exp.SelectModelUUID(gomock.Any(), knownID).Return(knownID, nil),
	)

	svc := NewService(s.state)

	known, err := svc.IsModelKnownToController(context.Background(), "is-not-there")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(known, jc.IsFalse)

	known, err = svc.IsModelKnownToController(context.Background(), knownID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(known, jc.IsTrue)
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}
