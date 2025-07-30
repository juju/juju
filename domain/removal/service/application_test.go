// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	applicationtesting "github.com/juju/juju/core/application/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	removal "github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

type applicationSuite struct {
	baseSuite
}

func TestApplicationSuite(t *testing.T) {
	tc.Run(t, &applicationSuite{})
}

func (s *applicationSuite) TestRemoveApplicationNoForceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.ApplicationExists(gomock.Any(), appUUID.String()).Return(true, nil)
	exp.EnsureApplicationNotAliveCascade(gomock.Any(), appUUID.String()).Return(nil, nil, nil)
	exp.ApplicationScheduleRemoval(gomock.Any(), gomock.Any(), appUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveApplication(c.Context(), appUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *applicationSuite) TestRemoveApplicationForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.ApplicationExists(gomock.Any(), appUUID.String()).Return(true, nil)
	exp.EnsureApplicationNotAliveCascade(gomock.Any(), appUUID.String()).Return(nil, nil, nil)
	exp.ApplicationScheduleRemoval(gomock.Any(), gomock.Any(), appUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveApplication(c.Context(), appUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *applicationSuite) TestRemoveApplicationForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.modelState.EXPECT()
	exp.ApplicationExists(gomock.Any(), appUUID.String()).Return(true, nil)
	exp.EnsureApplicationNotAliveCascade(gomock.Any(), appUUID.String()).Return(nil, nil, nil)

	// The first normal removal scheduled immediately.
	exp.ApplicationScheduleRemoval(gomock.Any(), gomock.Any(), appUUID.String(), false, when.UTC()).Return(nil)

	// The forced removal scheduled after the wait duration.
	exp.ApplicationScheduleRemoval(gomock.Any(), gomock.Any(), appUUID.String(), true, when.UTC().Add(time.Minute)).Return(nil)

	jobUUID, err := s.newService(c).RemoveApplication(c.Context(), appUUID, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *applicationSuite) TestRemoveApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.modelState.EXPECT().ApplicationExists(gomock.Any(), appUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveApplication(c.Context(), appUUID, false, 0)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationSuite) TestRemoveApplicationNoForceSuccessWithUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.ApplicationExists(gomock.Any(), appUUID.String()).Return(true, nil)
	exp.EnsureApplicationNotAliveCascade(gomock.Any(), appUUID.String()).Return([]string{"unit-1", "unit-2"}, nil, nil)
	exp.ApplicationScheduleRemoval(gomock.Any(), gomock.Any(), appUUID.String(), false, when.UTC()).Return(nil)

	// We don't want to create all the unit expectations here, so we'll assume
	// that the unit no longer exists, to prevent this test from depending on
	// the unit removal logic.
	exp.UnitExists(gomock.Any(), "unit-1").Return(false, nil)
	exp.UnitExists(gomock.Any(), "unit-2").Return(false, nil)

	jobUUID, err := s.newService(c).RemoveApplication(c.Context(), appUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *applicationSuite) TestRemoveApplicationNoForceSuccessWithMachines(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.ApplicationExists(gomock.Any(), appUUID.String()).Return(true, nil)
	exp.EnsureApplicationNotAliveCascade(gomock.Any(), appUUID.String()).Return(nil, []string{"machine-1", "machine-2"}, nil)
	exp.ApplicationScheduleRemoval(gomock.Any(), gomock.Any(), appUUID.String(), false, when.UTC()).Return(nil)

	// We don't want to create all the machine expectations here, so we'll
	// assume that the machine no longer exists, to prevent this test from
	// depending on the machine removal logic.
	exp.MachineExists(gomock.Any(), "machine-1").Return(false, nil)
	exp.MachineExists(gomock.Any(), "machine-2").Return(false, nil)

	jobUUID, err := s.newService(c).RemoveApplication(c.Context(), appUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *applicationSuite) TestProcessRemovalJobInvalidJobType(c *tc.C) {
	var invalidJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: invalidJobType,
	}

	err := s.newService(c).processApplicationRemovalJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *applicationSuite) TestExecuteJobForApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newApplicationJob(c)

	exp := s.modelState.EXPECT()
	exp.GetApplicationLife(gomock.Any(), j.EntityUUID).Return(-1, applicationerrors.ApplicationNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) TestExecuteJobForApplicationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newApplicationJob(c)

	exp := s.modelState.EXPECT()
	exp.GetApplicationLife(gomock.Any(), j.EntityUUID).Return(-1, errors.Errorf("the front fell off"))

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *applicationSuite) TestExecuteJobForApplicationStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newApplicationJob(c)

	exp := s.modelState.EXPECT()
	exp.GetApplicationLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *applicationSuite) TestExecuteJobForApplicationDyingDeleteApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newApplicationJob(c)

	exp := s.modelState.EXPECT()
	exp.GetApplicationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.DeleteApplication(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) TestExecuteJobForApplicationDyingDeleteApplicationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newApplicationJob(c)

	exp := s.modelState.EXPECT()
	exp.GetApplicationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.DeleteApplication(gomock.Any(), j.EntityUUID).Return(errors.Errorf("the front fell off"))

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func newApplicationJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.ApplicationJob,
		EntityUUID:  applicationtesting.GenApplicationUUID(c).String(),
	}
}
