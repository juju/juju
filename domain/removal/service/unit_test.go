// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	corecharmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/leadership"
	unit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	removal "github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/vault"
)

type unitSuite struct {
	baseSuite
}

func TestUnitSuite(t *testing.T) {
	tc.Run(t, &unitSuite{})
}

func (s *unitSuite) TestNewUnitRemovalJobsRequestedJobIsLast(c *tc.C) {
	now := time.Date(2026, time.November, 27, 12, 0, 0, 0, time.UTC)
	for _, jobType := range []removal.JobType{removal.UnitJob, removal.StorageAttachmentJob} {
		for _, test := range []struct {
			name  string
			force bool
			wait  time.Duration
			count int
		}{
			{name: "normal", count: 1},
			{name: "forced without wait", force: true, count: 1},
			{name: "forced with expired wait", force: true, wait: -time.Minute, count: 1},
			{name: "forced with smallest positive wait", force: true, wait: time.Nanosecond, count: 2},
			{name: "forced with wait", force: true, wait: time.Minute, count: 2},
		} {
			comment := tc.Commentf("%s: %s", jobType, test.name)
			jobs, err := newUnitRemovalJobs(jobType, "entity-uuid", test.force, test.wait, now)
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(jobs, tc.HasLen, test.count, comment)

			// The caller selects the last job as the primary unit job and
			// passes any preceding jobs as part of the cascade.
			requested := jobs[len(jobs)-1]
			c.Check(requested.Force, tc.Equals, test.force, comment)
			c.Check(requested.ScheduledFor, tc.Equals, now.Add(test.wait), comment)
			for _, job := range jobs {
				c.Check(job.UUID, tc.IsNonZeroUUID, comment)
				c.Check(job.RemovalType, tc.Equals, jobType, comment)
				c.Check(job.EntityUUID, tc.Equals, "entity-uuid", comment)
			}
			if test.count == 2 {
				immediate := jobs[0]
				c.Check(immediate.Force, tc.IsFalse, comment)
				c.Check(immediate.ScheduledFor, tc.Equals, now, comment)
				c.Check(immediate.UUID, tc.Not(tc.Equals), requested.UUID, comment)
			}
		}
	}
}

func (s *unitSuite) TestRemoveUnitNoForceMachineAndStorageSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)
	mUUID := "some-machine-uuid"
	saUUID := "some-storage-attachment-uuid"

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureUnitNotAliveCascade(gomock.Any(), uUUID.String(), false).Return(internal.CascadedUnitLives{
		MachineUUID: &mUUID,
		CascadedStorageLives: internal.CascadedStorageLives{
			CascadedStorageAttachmentLives: internal.CascadedStorageAttachmentLives{
				StorageAttachmentUUIDs: []string{saUUID},
			},
		},
	}, nil)
	s.expectUnitRemovalCascade(c, uUUID, false, when.UTC(), []internal.UnitRemovalJob{
		{RemovalType: removal.MachineJob, EntityUUID: mUUID, ScheduledFor: when.UTC()},
		{RemovalType: removal.StorageAttachmentJob, EntityUUID: saUUID, ScheduledFor: when.UTC()},
	})

	jobUUID, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(jobUUID, tc.Equals, removal.UUID(uUUID))
}

// TestRemoveUnitNoForceIgnoresWaitForCascadedJobs ensures that a
// non-forced unit removal ignores the wait duration for the whole
// cascade: the machine the unit was the last one on is scheduled
// immediately, not at now+wait.
func (s *unitSuite) TestRemoveUnitNoForceIgnoresWaitForCascadedJobs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)
	mUUID := "some-machine-uuid"

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureUnitNotAliveCascade(gomock.Any(), uUUID.String(), false).Return(internal.CascadedUnitLives{
		MachineUUID: &mUUID,
	}, nil)
	s.expectUnitRemovalCascade(c, uUUID, false, when.UTC(), []internal.UnitRemovalJob{
		{RemovalType: removal.MachineJob, EntityUUID: mUUID, ScheduledFor: when.UTC()},
	})

	jobUUID, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, false, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

// TestRemoveUnitForceWaitSchedulesMachineJobImmediatelyAndAfterWait
// ensures that a forced unit removal with a wait duration schedules
// the machine the unit was the last one on twice, like the unit
// itself: a normal removal job scheduled immediately, and a forced one
// scheduled after the wait duration.
func (s *unitSuite) TestRemoveUnitForceWaitSchedulesMachineJobImmediatelyAndAfterWait(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)
	mUUID := "some-machine-uuid"

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureUnitNotAliveCascade(gomock.Any(), uUUID.String(), false).Return(internal.CascadedUnitLives{
		MachineUUID: &mUUID,
	}, nil)
	s.expectUnitRemovalCascade(c, uUUID, true, when.UTC().Add(time.Minute), []internal.UnitRemovalJob{
		{RemovalType: removal.UnitJob, EntityUUID: uUUID.String(), ScheduledFor: when.UTC()},
		{RemovalType: removal.MachineJob, EntityUUID: mUUID, ScheduledFor: when.UTC()},
		{RemovalType: removal.MachineJob, EntityUUID: mUUID, Force: true, ScheduledFor: when.UTC().Add(time.Minute)},
	})

	jobUUID, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *unitSuite) TestRemoveUnitForceWaitSchedulesCharmRemovalOnce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)
	charmUUID := corecharmtesting.GenCharmID(c).String()

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureUnitNotAliveCascade(gomock.Any(), uUUID.String(), false).Return(internal.CascadedUnitLives{
		CharmUUID: &charmUUID,
	}, nil)
	s.expectUnitRemovalCascade(c, uUUID, true, when.UTC().Add(time.Minute), []internal.UnitRemovalJob{
		{RemovalType: removal.UnitJob, EntityUUID: uUUID.String(), ScheduledFor: when.UTC()},
		{RemovalType: removal.CharmJob, EntityUUID: charmUUID, ScheduledFor: when.UTC()},
	})

	jobUUID, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *unitSuite) TestRemoveUnitForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureUnitNotAliveCascade(gomock.Any(), uUUID.String(), false).Return(internal.CascadedUnitLives{}, nil)
	s.expectUnitRemovalCascade(c, uUUID, true, when.UTC(), nil)

	jobUUID, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *unitSuite) TestRemoveUnitForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureUnitNotAliveCascade(gomock.Any(), uUUID.String(), false).Return(internal.CascadedUnitLives{}, nil)
	s.expectUnitRemovalCascade(c, uUUID, true, when.UTC().Add(time.Minute), []internal.UnitRemovalJob{
		{RemovalType: removal.UnitJob, EntityUUID: uUUID.String(), ScheduledFor: when.UTC()},
	})

	jobUUID, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *unitSuite) TestRemoveUnitRetrySchedulesRemovalJobs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)
	mUUID := "machine-1"
	charmUUID := corecharmtesting.GenCharmID(c).String()
	when := time.Now()
	s.clock.EXPECT().Now().Return(when).AnyTimes()

	cascaded := internal.CascadedUnitLives{
		MachineUUID: &mUUID,
		CharmUUID:   &charmUUID,
		CascadedStorageLives: internal.CascadedStorageLives{
			CascadedStorageAttachmentLives: internal.CascadedStorageAttachmentLives{
				StorageAttachmentUUIDs:    []string{"storage-attachment-1"},
				FileSystemAttachmentUUIDs: []string{"filesystem-attachment-1"},
				VolumeAttachmentUUIDs:     []string{"volume-attachment-1"},
				VolumeAttachmentPlanUUIDs: []string{"volume-attachment-plan-1"},
			},
			CascadedStorageInstanceLives: internal.CascadedStorageInstanceLives{
				StorageInstanceUUIDs: []string{"storage-instance-1"},
				FileSystemUUIDs:      []string{"filesystem-1"},
				VolumeUUIDs:          []string{"volume-1"},
			},
		},
	}

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil).Times(2)
	exp.EnsureUnitNotAliveCascade(gomock.Any(), uUUID.String(), false).Return(cascaded, nil).Times(2)
	s.expectUnitRemovalCascade(c, uUUID, false, when.UTC(), []internal.UnitRemovalJob{
		{RemovalType: removal.MachineJob, EntityUUID: mUUID, ScheduledFor: when.UTC()},
		{RemovalType: removal.StorageAttachmentJob, EntityUUID: "storage-attachment-1", ScheduledFor: when.UTC()},
		{RemovalType: removal.StorageFilesystemAttachmentJob, EntityUUID: "filesystem-attachment-1", ScheduledFor: when.UTC()},
		{RemovalType: removal.StorageVolumeAttachmentJob, EntityUUID: "volume-attachment-1", ScheduledFor: when.UTC()},
		{RemovalType: removal.StorageVolumeAttachmentPlanJob, EntityUUID: "volume-attachment-plan-1", ScheduledFor: when.UTC()},
		{RemovalType: removal.StorageFilesystemJob, EntityUUID: "filesystem-1", ScheduledFor: when.UTC()},
		{RemovalType: removal.StorageVolumeJob, EntityUUID: "volume-1", ScheduledFor: when.UTC()},
		{RemovalType: removal.StorageInstanceJob, EntityUUID: "storage-instance-1", ScheduledFor: when.UTC()},
		{RemovalType: removal.CharmJob, EntityUUID: charmUUID, ScheduledFor: when.UTC()},
	}).Times(2)

	jobUUID1, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID1.Validate(), tc.ErrorIsNil)

	jobUUID2, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID2.Validate(), tc.ErrorIsNil)
}

func (s *unitSuite) TestRemoveUnitRetryWithForceSchedulesRemovalJobs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)
	mUUID := "machine-1"
	when := time.Now()
	s.clock.EXPECT().Now().Return(when).AnyTimes()

	cascaded := internal.CascadedUnitLives{
		MachineUUID: &mUUID,
		CascadedStorageLives: internal.CascadedStorageLives{
			CascadedStorageAttachmentLives: internal.CascadedStorageAttachmentLives{
				StorageAttachmentUUIDs:    []string{"storage-attachment-1"},
				FileSystemAttachmentUUIDs: []string{"filesystem-attachment-1"},
				VolumeAttachmentUUIDs:     []string{"volume-attachment-1"},
				VolumeAttachmentPlanUUIDs: []string{"volume-attachment-plan-1"},
			},
			CascadedStorageInstanceLives: internal.CascadedStorageInstanceLives{
				StorageInstanceUUIDs: []string{"storage-instance-1"},
				FileSystemUUIDs:      []string{"filesystem-1"},
				VolumeUUIDs:          []string{"volume-1"},
			},
		},
	}

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil).Times(2)
	exp.EnsureUnitNotAliveCascade(gomock.Any(), uUUID.String(), false).Return(cascaded, nil).Times(2)
	for _, force := range []bool{false, true} {
		s.expectUnitRemovalCascade(c, uUUID, force, when.UTC(), []internal.UnitRemovalJob{
			{RemovalType: removal.MachineJob, EntityUUID: mUUID, Force: force, ScheduledFor: when.UTC()},
			{RemovalType: removal.StorageAttachmentJob, EntityUUID: "storage-attachment-1", Force: force, ScheduledFor: when.UTC()},
			{RemovalType: removal.StorageFilesystemAttachmentJob, EntityUUID: "filesystem-attachment-1", Force: force, ScheduledFor: when.UTC()},
			{RemovalType: removal.StorageVolumeAttachmentJob, EntityUUID: "volume-attachment-1", Force: force, ScheduledFor: when.UTC()},
			{RemovalType: removal.StorageVolumeAttachmentPlanJob, EntityUUID: "volume-attachment-plan-1", Force: force, ScheduledFor: when.UTC()},
			{RemovalType: removal.StorageFilesystemJob, EntityUUID: "filesystem-1", Force: force, ScheduledFor: when.UTC()},
			{RemovalType: removal.StorageVolumeJob, EntityUUID: "volume-1", Force: force, ScheduledFor: when.UTC()},
			{RemovalType: removal.StorageInstanceJob, EntityUUID: "storage-instance-1", Force: force, ScheduledFor: when.UTC()},
		})
	}

	jobUUID1, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID1.Validate(), tc.ErrorIsNil)

	jobUUID2, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID2.Validate(), tc.ErrorIsNil)
}

func (s *unitSuite) TestRemoveUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)

	s.modelState.EXPECT().UnitExists(gomock.Any(), uUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, false, 0)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitSuite) TestRemoveUnitCascadeStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)
	siUUID := tc.Must(c, storage.NewStorageInstanceUUID)
	saUUID := tc.Must(c, storage.NewStorageAttachmentUUID)
	fsUUID := tc.Must(c, storage.NewFilesystemUUID)
	fsaUUID := tc.Must(c, storage.NewFilesystemAttachmentUUID)
	volUUID := tc.Must(c, storage.NewVolumeUUID)
	vaUUID := tc.Must(c, storage.NewVolumeAttachmentUUID)
	vapUUID := tc.Must(c, storage.NewVolumeAttachmentPlanUUID)

	when := time.Now().UTC()
	s.clock.EXPECT().Now().Return(when).AnyTimes()

	cascaded := internal.CascadedUnitLives{
		CascadedStorageLives: internal.CascadedStorageLives{
			CascadedStorageInstanceLives: internal.CascadedStorageInstanceLives{
				StorageInstanceUUIDs: []string{siUUID.String()},
				FileSystemUUIDs:      []string{fsUUID.String()},
				VolumeUUIDs:          []string{volUUID.String()},
			},
			CascadedStorageAttachmentLives: internal.CascadedStorageAttachmentLives{
				StorageAttachmentUUIDs:    []string{saUUID.String()},
				FileSystemAttachmentUUIDs: []string{fsaUUID.String()},
				VolumeAttachmentUUIDs:     []string{vaUUID.String()},
				VolumeAttachmentPlanUUIDs: []string{vapUUID.String()},
			},
		},
	}

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureUnitNotAliveCascade(
		gomock.Any(), uUUID.String(), false,
	).Return(cascaded, nil)
	s.expectUnitRemovalCascade(c, uUUID, false, when, []internal.UnitRemovalJob{
		{RemovalType: removal.StorageInstanceJob, EntityUUID: siUUID.String(), ScheduledFor: when},
		{RemovalType: removal.StorageAttachmentJob, EntityUUID: saUUID.String(), ScheduledFor: when},
		{RemovalType: removal.StorageFilesystemJob, EntityUUID: fsUUID.String(), ScheduledFor: when},
		{RemovalType: removal.StorageFilesystemAttachmentJob, EntityUUID: fsaUUID.String(), ScheduledFor: when},
		{RemovalType: removal.StorageVolumeJob, EntityUUID: volUUID.String(), ScheduledFor: when},
		{RemovalType: removal.StorageVolumeAttachmentJob, EntityUUID: vaUUID.String(), ScheduledFor: when},
		{RemovalType: removal.StorageVolumeAttachmentPlanJob, EntityUUID: vapUUID.String(), ScheduledFor: when},
	})

	svc := s.newService(c)
	jobUUID, err := svc.RemoveUnit(c.Context(), uUUID, false, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *unitSuite) TestRemoveUnitDoesNotRevokeLeadership(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)
	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureUnitNotAliveCascade(gomock.Any(), uUUID.String(), false).Return(internal.CascadedUnitLives{}, nil)
	s.expectUnitRemovalCascade(c, uUUID, false, when.UTC(), nil)

	jobUUID, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *unitSuite) expectUnitRemovalCascade(
	c *tc.C, unitUUID unit.UUID, force bool, when time.Time,
	expected []internal.UnitRemovalJob,
) *gomock.FinalizedCall6[context.Context, string, string, bool, time.Time, []internal.UnitRemovalJob] {
	return s.modelState.EXPECT().UnitScheduleRemovalCascade(
		gomock.Any(), tc.Bind(tc.IsNonZeroUUID), unitUUID.String(), force, when, gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, _ string, _ bool, _ time.Time, jobs []internal.UnitRemovalJob) (string, error) {
		actual := append([]internal.UnitRemovalJob(nil), jobs...)
		for i := range actual {
			c.Check(actual[i].UUID, tc.IsNonZeroUUID)
			actual[i].UUID = ""
		}
		c.Check(actual, tc.SameContents, expected)
		return unitUUID.String(), nil
	})
}

func (s *unitSuite) TestRemoveUnitSchedulingFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()
	uUUID := unittesting.GenUnitUUID(c)
	when := time.Now().UTC()
	s.clock.EXPECT().Now().Return(when)
	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureUnitNotAliveCascade(gomock.Any(), uUUID.String(), false).Return(internal.CascadedUnitLives{}, nil)
	schedulingErr := errors.New("scheduling failed")
	exp.UnitScheduleRemovalCascade(gomock.Any(), gomock.Any(), uUUID.String(), false, when, gomock.Any()).Return("", schedulingErr)

	_, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, false, 0)
	c.Assert(err, tc.ErrorIs, schedulingErr)
}

func (s *unitSuite) TestRemoveUnitInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.newService(c).RemoveUnit(c.Context(), unit.UUID("invalid"), false, false, 0)
	c.Check(err, tc.ErrorMatches, "validating unit UUID: .*not valid.*")
}

func (s *unitSuite) TestProcessRemovalJobInvalidJobType(c *tc.C) {
	var invalidJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: invalidJobType,
	}

	err := s.newService(c).processUnitRemovalJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *unitSuite) TestExecuteJobForUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(-1, applicationerrors.UnitNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestExecuteJobForUnitError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(-1, errors.Errorf("the front fell off"))

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *unitSuite) TestExecuteJobForUnitStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *unitSuite) TestExecuteJobForUnitDeadDeleteUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.GetApplicationNameAndUnitNameByUnitUUID(gomock.Any(), j.EntityUUID).Return("foo", "foo/0", nil)
	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(nil)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID, false).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return(sbCfg, nil)
	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestExecuteJobForUnitDeadDyingUnitWithNoEntities(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.MarkUnitAsDeadWithNoEntities(gomock.Any(), j.EntityUUID).Return(nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.GetApplicationNameAndUnitNameByUnitUUID(gomock.Any(), j.EntityUUID).Return("foo", "foo/0", nil)
	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(nil)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID, false).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return(sbCfg, nil)
	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestExecuteJobForUnitDeadJujuSecretsDeleteUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.GetApplicationNameAndUnitNameByUnitUUID(gomock.Any(), j.EntityUUID).Return("foo", "foo/0", nil)
	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(nil)
	exp.DeleteUnitOwnedSecretContent(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID, false).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: juju.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return(sbCfg, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestExecuteJobForUnitDeadExternalSecretsDeleteUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	secretExternalRefs := []string{"wun", "too", "free"}

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.GetApplicationNameAndUnitNameByUnitUUID(gomock.Any(), j.EntityUUID).Return("foo", "foo/0", nil)
	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(nil)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(secretExternalRefs, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID, false).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return(sbCfg, nil)

	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)
	for i, id := range secretExternalRefs {
		var err error
		if i == 0 {
			err = errors.New("no matter; we only log it as a warning")
		}
		s.secretBackend.EXPECT().DeleteContent(c.Context(), id).Return(err)
	}

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestExecuteJobWithForceRevokesLeadershipAfterLeavingScopes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ruUUID := "some-relation-unit"

	j := newUnitJob(c)
	j.Force = true

	exp := s.modelState.EXPECT()
	gomock.InOrder(
		exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil),
		exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return([]string{ruUUID}, nil),
		exp.LeaveScope(gomock.Any(), ruUUID).Return(nil),
		exp.GetApplicationNameAndUnitNameByUnitUUID(gomock.Any(), j.EntityUUID).Return("foo", "foo/0", nil),
		s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(nil),
	)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID, true).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return(sbCfg, nil)
	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestExecuteJobLeaveScopeErrorDoesNotRevokeLeadership(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)
	j.Force = true
	ruUUID := "some-relation-unit"

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return([]string{ruUUID}, nil)
	exp.LeaveScope(gomock.Any(), ruUUID).Return(errors.New("cannot leave scope"))

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, `removing relation unit .*: cannot leave scope`)
}

func (s *unitSuite) TestExecuteJobForUnitDeadDeleteUnitError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.GetApplicationNameAndUnitNameByUnitUUID(gomock.Any(), j.EntityUUID).Return("foo", "foo/0", nil)
	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(nil)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID, false).Return(errors.Errorf("the front fell off"))

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return(sbCfg, nil)
	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *unitSuite) TestExecuteJobForDyingUnitWithLiveEntitiesDoesNotRevokeLeadership(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.MarkUnitAsDeadWithNoEntities(gomock.Any(), j.EntityUUID).Return(removalerrors.EntityStillAlive)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityNotDead)
}

func (s *unitSuite) TestExecuteJobForUnitLeadershipRevocationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.GetApplicationNameAndUnitNameByUnitUUID(gomock.Any(), j.EntityUUID).Return("foo", "foo/0", nil)
	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(errors.New("lease unavailable"))

	// No destructive cleanup is expected when leadership cannot be revoked.
	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, `revoking leadership for unit .*: lease unavailable`)
}

func (s *unitSuite) TestExecuteJobForUnitDeadDeleteUnitClaimNotHeld(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.GetApplicationNameAndUnitNameByUnitUUID(gomock.Any(), j.EntityUUID).Return("foo", "foo/0", nil)
	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(leadership.ErrClaimNotHeld)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID, false).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return(sbCfg, nil)
	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestMarkUnitAsDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.MarkUnitAsDead(gomock.Any(), uUUID.String()).Return(nil)

	err := s.newService(c).MarkUnitAsDead(c.Context(), uUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestMarkUnitAsDeadUnitDoesNotExist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(false, nil)

	err := s.newService(c).MarkUnitAsDead(c.Context(), uUUID)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitSuite) TestMarkUnitAsDeadError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)

	exp := s.modelState.EXPECT()
	exp.UnitExists(gomock.Any(), uUUID.String()).Return(false, errors.Errorf("the front fell off"))

	err := s.newService(c).MarkUnitAsDead(c.Context(), uUUID)
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
