// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/vault"
)

type secretSuite struct {
	baseSuite
}

func TestSecretSuite(t *testing.T) {
	tc.Run(t, &secretSuite{})
}

func (s *secretSuite) TestProcessRemovalJobInvalidJobType(c *tc.C) {
	job := removal.Job{RemovalType: 500}
	err := s.newService(c).processUserSecretRemovalJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *secretSuite) TestProcessUserSecretRemovalJobInvalidArgs(c *tc.C) {
	j := removal.Job{
		UUID:        func() removal.UUID { u, _ := removal.NewUUID(); return u }(),
		RemovalType: removal.UserSecretJob,
		EntityUUID:  secrets.NewURI().String(),
		Arg: map[string]any{
			"revisions": "not-a-valid-type",
		},
	}
	err := s.newService(c).processUserSecretRemovalJob(c.Context(), j)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobArgsInvalid)
}

func (s *secretSuite) TestProcessUserSecretRemovalJobInvalidRevisions(c *tc.C) {
	j := removal.Job{
		UUID:        func() removal.UUID { u, _ := removal.NewUUID(); return u }(),
		RemovalType: removal.UserSecretJob,
		EntityUUID:  secrets.NewURI().String(),
		Arg: map[string]any{
			"revisions": []any{"invalid-revisions"},
		},
	}
	err := s.newService(c).processUserSecretRemovalJob(c.Context(), j)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobArgsInvalid)
}

func (s *secretSuite) TestExecuteJobForUserSecretWithInvalidArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := removal.Job{
		UUID:        func() removal.UUID { u, _ := removal.NewUUID(); return u }(),
		RemovalType: removal.UserSecretJob,
		EntityUUID:  secrets.NewURI().String(),
		Arg: map[string]any{
			"revisions": "not-a-valid-type",
		},
	}

	// The job is non-retryable, so it must be deleted from the model state.
	s.modelState.EXPECT().DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *secretSuite) TestExecuteJobForUserSecretDelete(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUserSecretJob(c)
	uri, _ := secrets.ParseURI(j.EntityUUID)

	exp := s.modelState.EXPECT()
	exp.DeleteUserSecretRevisions(gomock.Any(), uri, nil).Return([]string{"rev-uuid-1"}, nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: juju.BackendType,
		},
	}
	s.controllerState.EXPECT().
		GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).
		Return("", sbCfg, nil)
	s.controllerState.EXPECT().RemoveSecretBackendReference(gomock.Any(), "rev-uuid-1").Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *secretSuite) TestExecuteJobForUserSecretWithSpecificRevisions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUserSecretJobWithRevisions(c, []int{1, 2})
	uri, _ := secrets.ParseURI(j.EntityUUID)

	revs := []string{"rev-uuid-1", "rev-uuid-2"}

	exp := s.modelState.EXPECT()
	exp.DeleteUserSecretRevisions(gomock.Any(), uri, []int{1, 2}).Return(revs, nil)
	exp.GetUserSecretRevisionRefs(gomock.Any(), revs).Return([]string{"ref-1", "ref-2"}, nil)
	exp.DeleteUserSecretRevisionRef(gomock.Any(), "ref-1").Return(nil)
	exp.DeleteUserSecretRevisionRef(gomock.Any(), "ref-2").Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	s.prepareSecretBackendProvider()
	s.secretBackend.EXPECT().DeleteContent(c.Context(), "ref-1").Return(nil)
	s.secretBackend.EXPECT().DeleteContent(c.Context(), "ref-2").Return(nil)

	s.controllerState.EXPECT().RemoveSecretBackendReference(gomock.Any(), "rev-uuid-1", "rev-uuid-2").Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *secretSuite) TestExecuteJobForUserSecretExternalSecretsDelete(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUserSecretJob(c)
	uri, _ := secrets.ParseURI(j.EntityUUID)

	secretExternalRefs := []string{"ref-1", "ref-2", "ref-3"}
	deletedRevisionUUIDs := []string{"rev-uuid-1", "rev-uuid-2", "rev-uuid-3"}

	exp := s.modelState.EXPECT()
	exp.DeleteUserSecretRevisions(gomock.Any(), uri, nil).Return(deletedRevisionUUIDs, nil)
	exp.GetUserSecretRevisionRefs(gomock.Any(), deletedRevisionUUIDs).Return(secretExternalRefs, nil)
	exp.DeleteUserSecretRevisionRef(gomock.Any(), secretExternalRefs[0]).Return(nil)
	exp.DeleteUserSecretRevisionRef(gomock.Any(), secretExternalRefs[1]).Return(nil)
	exp.DeleteUserSecretRevisionRef(gomock.Any(), secretExternalRefs[2]).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	s.prepareSecretBackendProvider()
	for _, id := range secretExternalRefs {
		s.secretBackend.EXPECT().DeleteContent(c.Context(), id).Return(nil)
	}

	s.controllerState.EXPECT().
		RemoveSecretBackendReference(gomock.Any(), "rev-uuid-1", "rev-uuid-2", "rev-uuid-3").
		Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *secretSuite) TestExecuteJobForUserSecretExternalSecretsDeleteWithFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newUserSecretJob(c)
	uri, _ := secrets.ParseURI(j.EntityUUID)

	secretExternalRefs := []string{"ref-1", "ref-2"}
	deletedRevisionUUIDs := []string{"rev-uuid-1", "rev-uuid-2"}

	exp := s.modelState.EXPECT()
	exp.DeleteUserSecretRevisions(gomock.Any(), uri, nil).Return(deletedRevisionUUIDs, nil)
	exp.GetUserSecretRevisionRefs(gomock.Any(), deletedRevisionUUIDs).Return(secretExternalRefs, nil)
	exp.DeleteUserSecretRevisionRef(gomock.Any(), secretExternalRefs[0]).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	s.prepareSecretBackendProvider()
	s.secretBackend.EXPECT().DeleteContent(c.Context(), secretExternalRefs[0]).Return(nil)
	s.secretBackend.EXPECT().DeleteContent(c.Context(), secretExternalRefs[1]).Return(errors.Errorf("backend error"))

	s.controllerState.EXPECT().RemoveSecretBackendReference(gomock.Any(), "rev-uuid-1", "rev-uuid-2").Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func newUserSecretJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.UserSecretJob,
		EntityUUID:  secrets.NewURI().String(),
	}
}

func newUserSecretJobWithRevisions(c *tc.C, revisions []int) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.UserSecretJob,
		EntityUUID:  secrets.NewURI().String(),
		Arg: map[string]any{
			"revisions": revisions,
		},
	}
}

func (s *secretSuite) prepareSecretBackendProvider() {
	sbCfg := &provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
		},
	}
	s.controllerState.EXPECT().
		GetActiveModelSecretBackend(gomock.Any(), s.modelUUID.String()).
		Return("", sbCfg, nil)

	s.secretBackendProvider.EXPECT().Initialise(sbCfg).Return(nil)
	s.secretBackendProvider.EXPECT().NewBackend(sbCfg).Return(s.secretBackend, nil)
}
