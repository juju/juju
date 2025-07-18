// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

type modelSuite struct {
	baseSuite
}

func TestModelSuite(t *testing.T) {
	tc.Run(t, &modelSuite{})
}

func (s *modelSuite) TestRemoveModelNoForceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := modeltesting.GenModelUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	exp.EnsureModelNotAliveCascade(gomock.Any(), mUUID.String(), false).Return(removal.ModelArtifacts{
		RelationUUIDs:    []string{"some-relation-id"},
		UnitUUIDs:        []string{"some-unit-id"},
		MachineUUIDs:     []string{"some-machine-id"},
		ApplicationUUIDs: []string{"some-application-id"},
	}, nil)
	exp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), false, when.UTC()).Return(nil)

	// We don't want to create all the machine, unit, relation and application
	// expectations here, so we'll assume that the
	// machine/unit/relation/application no longer exists, to prevent this test
	// from depending on the machine/unit/relation/application removal logic.
	exp.MachineExists(gomock.Any(), "some-machine-id").Return(false, nil)
	exp.UnitExists(gomock.Any(), "some-unit-id").Return(false, nil)
	exp.RelationExists(gomock.Any(), "some-relation-id").Return(false, nil)
	exp.ApplicationExists(gomock.Any(), "some-application-id").Return(false, nil)

	jobUUID, err := s.newService(c).RemoveModel(c.Context(), mUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *modelSuite) TestRemoveModelForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := modeltesting.GenModelUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	exp.EnsureModelNotAliveCascade(gomock.Any(), mUUID.String(), true).Return(removal.ModelArtifacts{}, nil)
	exp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveModel(c.Context(), mUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *modelSuite) TestRemoveModelForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := modeltesting.GenModelUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.state.EXPECT()
	exp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	exp.EnsureModelNotAliveCascade(gomock.Any(), mUUID.String(), true).Return(removal.ModelArtifacts{}, nil)

	// The first normal removal scheduled immediately.
	exp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), false, when.UTC()).Return(nil)

	// The forced removal scheduled after the wait duration.
	exp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), true, when.UTC().Add(time.Minute)).Return(nil)

	jobUUID, err := s.newService(c).RemoveModel(c.Context(), mUUID, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *modelSuite) TestRemoveModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := modeltesting.GenModelUUID(c)

	s.state.EXPECT().ModelExists(gomock.Any(), mUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveModel(c.Context(), mUUID, false, 0)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestProcessRemovalJobInvalidJobType(c *tc.C) {
	var invalidJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: invalidJobType,
	}

	err := s.newService(c).processModelRemovalJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *modelSuite) TestExecuteJobForModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// This does not return model not found, instead it returns that the
	// model was removed successfully, which is a different error. That way
	// we can ensure anyone that is listening for removal jobs will
	// be able to handle the removal of the model.

	j := newModelJob(c)

	exp := s.state.EXPECT()
	exp.GetModelLife(gomock.Any(), j.EntityUUID).Return(-1, modelerrors.NotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalModelRemoved)
}

func (s *modelSuite) TestExecuteJobForModelError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	exp := s.state.EXPECT()
	exp.GetModelLife(gomock.Any(), j.EntityUUID).Return(-1, errors.Errorf("the front fell off"))

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *modelSuite) TestExecuteJobForModelStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	exp := s.state.EXPECT()
	exp.GetModelLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func newModelJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.ModelJob,
		EntityUUID:  modeltesting.GenModelUUID(c).String(),
	}
}
