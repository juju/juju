// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corerelation "github.com/juju/juju/core/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	jujutesting.IsolationSuite

	state *MockState
	clock *MockClock
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestRemoveRelationNoForceSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.RelationExists(gomock.Any(), rUUID.String()).Return(true, nil)
	exp.RelationAdvanceLifeAndScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelation(context.Background(), rUUID, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), jc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveRelationForceSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.RelationExists(gomock.Any(), rUUID.String()).Return(true, nil)
	exp.RelationAdvanceLifeAndScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelation(context.Background(), rUUID, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), jc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveRelationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	s.state.EXPECT().RelationExists(gomock.Any(), rUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveRelation(context.Background(), rUUID, true)
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.clock = NewMockClock(ctrl)

	return ctrl
}

func (s *serviceSuite) newService(c *gc.C) *Service {
	return &Service{
		st:     s.state,
		clock:  s.clock,
		logger: loggertesting.WrapCheckLog(c),
	}
}

func newRelUUID(c *gc.C) corerelation.UUID {
	rUUID, err := corerelation.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return rUUID
}
