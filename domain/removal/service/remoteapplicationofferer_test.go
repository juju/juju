// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	"github.com/juju/juju/internal/errors"
)

type remoteApplicationOffererSuite struct {
	baseSuite
}

func TestRemoteApplicationOffererSuite(t *testing.T) {
	tc.Run(t, &remoteApplicationOffererSuite{})
}

func (s *remoteApplicationOffererSuite) TestRemoveRemoteApplicationOffererByApplicationUUIDNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	remoteAppUUID := tc.Must(c, coreremoteapplication.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.GetRemoteApplicationOffererUUIDByApplicationUUID(gomock.Any(), appUUID.String()).Return(remoteAppUUID.String(), nil)
	exp.RemoteApplicationOffererExists(gomock.Any(), remoteAppUUID.String()).Return(true, nil)
	exp.EnsureRemoteApplicationOffererNotAliveCascade(gomock.Any(), remoteAppUUID.String()).Return(internal.CascadedRemoteApplicationOffererLives{}, nil)
	exp.RemoteApplicationOffererScheduleRemoval(gomock.Any(), gomock.Any(), remoteAppUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRemoteApplicationOffererByApplicationUUID(c.Context(), appUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *remoteApplicationOffererSuite) TestRemoveRemoteApplicationOffererNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	remoteAppUUID := tc.Must(c, coreremoteapplication.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.RemoteApplicationOffererExists(gomock.Any(), remoteAppUUID.String()).Return(true, nil)
	exp.EnsureRemoteApplicationOffererNotAliveCascade(gomock.Any(), remoteAppUUID.String()).Return(internal.CascadedRemoteApplicationOffererLives{}, nil)
	exp.RemoteApplicationOffererScheduleRemoval(gomock.Any(), gomock.Any(), remoteAppUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRemoteApplicationOfferer(c.Context(), remoteAppUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *remoteApplicationOffererSuite) TestRemoveRemoteApplicationOffererForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	remoteAppUUID := tc.Must(c, coreremoteapplication.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.modelState.EXPECT()
	exp.RemoteApplicationOffererExists(gomock.Any(), remoteAppUUID.String()).Return(true, nil)
	exp.EnsureRemoteApplicationOffererNotAliveCascade(gomock.Any(), remoteAppUUID.String()).Return(internal.CascadedRemoteApplicationOffererLives{}, nil)

	// The first normal removal scheduled immediately.
	exp.RemoteApplicationOffererScheduleRemoval(gomock.Any(), gomock.Any(), remoteAppUUID.String(), false, when.UTC()).Return(nil)

	// The forced removal scheduled after the wait duration.
	exp.RemoteApplicationOffererScheduleRemoval(gomock.Any(), gomock.Any(), remoteAppUUID.String(), true, when.UTC().Add(time.Minute)).Return(nil)

	jobUUID, err := s.newService(c).RemoveRemoteApplicationOfferer(c.Context(), remoteAppUUID, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *remoteApplicationOffererSuite) TestRemoveRemoteApplicationOffererNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	remoteAppUUID := tc.Must(c, coreremoteapplication.NewUUID)

	s.modelState.EXPECT().RemoteApplicationOffererExists(gomock.Any(), remoteAppUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveRemoteApplicationOfferer(c.Context(), remoteAppUUID, false, 0)
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RemoteApplicationNotFound)
}

func (s *remoteApplicationOffererSuite) TestRemoveRemoteApplicationOffererNoForceSuccessWithRelations(c *tc.C) {
	defer s.setupMocks(c).Finish()

	remoteAppUUID := tc.Must(c, coreremoteapplication.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.modelState.EXPECT()
	exp.RemoteApplicationOffererExists(gomock.Any(), remoteAppUUID.String()).Return(true, nil)
	exp.EnsureRemoteApplicationOffererNotAliveCascade(gomock.Any(), remoteAppUUID.String()).Return(internal.CascadedRemoteApplicationOffererLives{
		RelationUUIDs: []string{"relation-1", "relation-2"},
	}, nil)
	exp.RemoteApplicationOffererScheduleRemoval(gomock.Any(), gomock.Any(), remoteAppUUID.String(), false, when.UTC()).Return(nil)

	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), "relation-1", false, when.UTC()).Return(nil)
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), "relation-2", false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRemoteApplicationOfferer(c.Context(), remoteAppUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *remoteApplicationOffererSuite) TestProcessRemovalJobInvalidJobType(c *tc.C) {
	var invalidJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: invalidJobType,
	}

	err := s.newService(c).processRemoteApplicationOffererRemovalJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *remoteApplicationOffererSuite) TestExecuteJobForRemoteApplicationOffererNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRemoteApplicationOffererJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRemoteApplicationOffererLife(gomock.Any(), j.EntityUUID).Return(-1, crossmodelrelationerrors.RemoteApplicationNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteApplicationOffererSuite) TestExecuteJobForRemoteApplicationOffererError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRemoteApplicationOffererJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRemoteApplicationOffererLife(gomock.Any(), j.EntityUUID).Return(-1, errors.Errorf("the front fell off"))

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *remoteApplicationOffererSuite) TestExecuteJobForRemoteApplicationOffererStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRemoteApplicationOffererJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRemoteApplicationOffererLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *remoteApplicationOffererSuite) TestExecuteJobForRemoteApplicationOffererDyingDeleteRemoteApplicationOfferer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRemoteApplicationOffererJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRemoteApplicationOffererLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.DeleteRemoteApplicationOfferer(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteApplicationOffererSuite) TestExecuteJobForRemoteApplicationOffererDyingDeleteRemoteApplicationOffererError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRemoteApplicationOffererJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRemoteApplicationOffererLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.DeleteRemoteApplicationOfferer(gomock.Any(), j.EntityUUID).Return(errors.Errorf("the front fell off"))

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func newRemoteApplicationOffererJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.RemoteApplicationOffererJob,
		EntityUUID:  tc.Must(c, coreremoteapplication.NewUUID).String(),
	}
}
