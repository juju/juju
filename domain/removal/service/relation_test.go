// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corerelation "github.com/juju/juju/core/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
)

type relationSuite struct {
	baseSuite
}

var _ = gc.Suite(&relationSuite{})

func (s *relationSuite) TestRemoveRelationNoForceSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.RelationExists(gomock.Any(), rUUID.String()).Return(true, nil)
	exp.RelationAdvanceLife(gomock.Any(), rUUID.String()).Return(nil)
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelation(context.Background(), rUUID, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), jc.ErrorIsNil)
}

func (s *relationSuite) TestRemoveRelationForceSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.RelationExists(gomock.Any(), rUUID.String()).Return(true, nil)
	exp.RelationAdvanceLife(gomock.Any(), rUUID.String()).Return(nil)
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelation(context.Background(), rUUID, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), jc.ErrorIsNil)
}

func (s *relationSuite) TestRemoveRelationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	s.state.EXPECT().RelationExists(gomock.Any(), rUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveRelation(context.Background(), rUUID, true)
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}
func newRelUUID(c *gc.C) corerelation.UUID {
	rUUID, err := corerelation.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return rUUID
}
