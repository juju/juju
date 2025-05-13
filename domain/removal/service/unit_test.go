// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
)

type unitSuite struct {
	baseSuite
}

var _ = tc.Suite(&unitSuite{})

func (s *unitSuite) TestRemoveUnitNoForceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureUnitNotAlive(gomock.Any(), uUUID.String()).Return(nil)
	exp.UnitScheduleRemoval(gomock.Any(), gomock.Any(), uUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveUnit(context.Background(), uUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *unitSuite) TestRemoveUnitForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureUnitNotAlive(gomock.Any(), uUUID.String()).Return(nil)
	exp.UnitScheduleRemoval(gomock.Any(), gomock.Any(), uUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveUnit(context.Background(), uUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *unitSuite) TestRemoveUnitForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.state.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureUnitNotAlive(gomock.Any(), uUUID.String()).Return(nil)

	// The first normal removal scheduled immediately.
	exp.UnitScheduleRemoval(gomock.Any(), gomock.Any(), uUUID.String(), false, when.UTC()).Return(nil)

	// The forced removal scheduled after the wait duration.
	exp.UnitScheduleRemoval(gomock.Any(), gomock.Any(), uUUID.String(), true, when.UTC().Add(time.Minute)).Return(nil)

	jobUUID, err := s.newService(c).RemoveUnit(context.Background(), uUUID, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *unitSuite) TestRemoveUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().UnitExists(gomock.Any(), uUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveUnit(context.Background(), uUUID, false, 0)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}
