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
	"github.com/juju/juju/domain/life"
	removal "github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
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
	exp.EnsureUnitNotAlive(gomock.Any(), uUUID.String()).Return("some-machine-id", nil)
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
	exp.EnsureUnitNotAlive(gomock.Any(), uUUID.String()).Return("", nil)
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
	exp.EnsureUnitNotAlive(gomock.Any(), uUUID.String()).Return("", nil)

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

func (s *unitSuite) TestProcessRemovalJobInvalidJobType(c *tc.C) {
	var invalidJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: invalidJobType,
	}

	err := s.newService(c).processUnitRemovalJob(context.Background(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *unitSuite) TestExecuteJobForUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.state.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(-1, applicationerrors.UnitNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(context.Background(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestExecuteJobForUnitError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.state.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(-1, errors.Errorf("the front fell off"))

	err := s.newService(c).ExecuteJob(context.Background(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *unitSuite) TestExecuteJobForUnitStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.state.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(context.Background(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *unitSuite) TestExecuteJobForUnitDyingDeleteUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.state.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(context.Background(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestExecuteJobForUnitDyingDeleteUnitError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.state.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID).Return(errors.Errorf("the front fell off"))

	err := s.newService(c).ExecuteJob(context.Background(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func newUnitJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.UnitJob,
		EntityUUID:  unittesting.GenUnitUUID(c).String(),
	}
}
