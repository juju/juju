// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/leadership"
	unit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	removal "github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
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
			StorageAttachmentUUIDs: []string{saUUID},
		},
	}, nil)
	exp.UnitScheduleRemoval(gomock.Any(), gomock.Any(), uUUID.String(), false, when.UTC()).Return(nil)
	exp.MachineScheduleRemoval(gomock.Any(), gomock.Any(), mUUID, false, when.UTC()).Return(nil)
	exp.StorageAttachmentScheduleRemoval(gomock.Any(), gomock.Any(), saUUID, false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, false, 0)
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
	exp.UnitScheduleRemoval(gomock.Any(), gomock.Any(), uUUID.String(), true, when.UTC()).Return(nil)

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

	// The first normal removal scheduled immediately.
	exp.UnitScheduleRemoval(gomock.Any(), gomock.Any(), uUUID.String(), false, when.UTC()).Return(nil)

	// The forced removal scheduled after the wait duration.
	exp.UnitScheduleRemoval(gomock.Any(), gomock.Any(), uUUID.String(), true, when.UTC().Add(time.Minute)).Return(nil)

	jobUUID, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *unitSuite) TestRemoveUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := unittesting.GenUnitUUID(c)

	s.modelState.EXPECT().UnitExists(gomock.Any(), uUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveUnit(c.Context(), uUUID, false, false, 0)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
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
	exp.GetCharmForUnit(gomock.Any(), j.EntityUUID).Return(tc.Must(c, unit.NewUUID).String(), nil)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteCharmIfUnused(gomock.Any(), gomock.Any()).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return("", sbCfg, nil)
	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)

	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(nil)

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
	exp.GetCharmForUnit(gomock.Any(), j.EntityUUID).Return(tc.Must(c, unit.NewUUID).String(), nil)
	exp.DeleteUnitOwnedSecretContent(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteCharmIfUnused(gomock.Any(), gomock.Any()).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: juju.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return("", sbCfg, nil)

	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(nil)

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
	exp.GetCharmForUnit(gomock.Any(), j.EntityUUID).Return(tc.Must(c, unit.NewUUID).String(), nil)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(secretExternalRefs, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteCharmIfUnused(gomock.Any(), gomock.Any()).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return("", sbCfg, nil)

	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)
	for i, id := range secretExternalRefs {
		var err error
		if i == 0 {
			err = errors.New("no matter; we only log it as a warning")
		}
		s.secretBackend.EXPECT().DeleteContent(c.Context(), id).Return(err)
	}

	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestExecuteJobWithForceForUnitDyingDeleteUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ruUUID := "some-relation-unit"

	j := newUnitJob(c)
	j.Force = true

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.GetApplicationNameAndUnitNameByUnitUUID(gomock.Any(), j.EntityUUID).Return("foo", "foo/0", nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return([]string{ruUUID}, nil)
	exp.LeaveScope(gomock.Any(), ruUUID).Return(nil)
	exp.GetCharmForUnit(gomock.Any(), j.EntityUUID).Return(tc.Must(c, unit.NewUUID).String(), nil)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteCharmIfUnused(gomock.Any(), gomock.Any()).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return("", sbCfg, nil)
	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)

	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestExecuteJobForUnitDeadDeleteUnitError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.GetCharmForUnit(gomock.Any(), j.EntityUUID).Return(tc.Must(c, unit.NewUUID).String(), nil)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID).Return(errors.Errorf("the front fell off"))

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return("", sbCfg, nil)
	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *unitSuite) TestDeleteCharmForUnitFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.GetApplicationNameAndUnitNameByUnitUUID(gomock.Any(), j.EntityUUID).Return("foo", "foo/0", nil)
	exp.GetCharmForUnit(gomock.Any(), j.EntityUUID).Return(tc.Must(c, unit.NewUUID).String(), nil)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteCharmIfUnused(gomock.Any(), gomock.Any()).Return(errors.Errorf("the charm is still in use"))
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return("", sbCfg, nil)
	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)

	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestExecuteJobForUnitNotDeadError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityNotDead)
}

func (s *unitSuite) TestExecuteJobForUnitRevokingUnitError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.GetCharmForUnit(gomock.Any(), j.EntityUUID).Return(tc.Must(c, unit.NewUUID).String(), nil)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteCharmIfUnused(gomock.Any(), gomock.Any()).Return(nil)
	exp.GetApplicationNameAndUnitNameByUnitUUID(gomock.Any(), j.EntityUUID).Return("foo", "foo/0", nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return("", sbCfg, nil)
	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)

	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(errors.Errorf("the front fell off"))

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *unitSuite) TestExecuteJobForUnitDeadDeleteUnitClaimNotHeld(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUnitJob(c)

	exp := s.modelState.EXPECT()
	exp.GetUnitLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetRelationUnitsForUnit(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.GetApplicationNameAndUnitNameByUnitUUID(gomock.Any(), j.EntityUUID).Return("foo", "foo/0", nil)
	exp.GetCharmForUnit(gomock.Any(), j.EntityUUID).Return(tc.Must(c, unit.NewUUID).String(), nil)
	exp.GetUnitOwnedSecretRevisionRefs(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteUnitOwnedSecrets(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteUnit(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteCharmIfUnused(gomock.Any(), gomock.Any()).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).Return("", sbCfg, nil)
	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)

	s.revoker.EXPECT().RevokeLeadership("foo", unit.Name("foo/0")).Return(leadership.ErrClaimNotHeld)

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
