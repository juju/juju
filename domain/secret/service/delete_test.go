// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/uuid"
)

func (s *serviceSuite) TestDeleteSecretInternal(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	revisionID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().ListExternalSecretRevisions(gomock.Any(), uri, 666).Return([]coresecrets.ValueRef{}, nil)
	s.state.EXPECT().DeleteSecret(gomock.Any(), uri, []int{666}).Return([]uuid.UUID{revisionID}, nil)
	s.secretBackendReferenceMutator.EXPECT().RemoveSecretBackendReference(gomock.Any(), revisionID).Return(nil)

	err = s.service.DeleteSecret(context.Background(), uri, DeleteSecretParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{666},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteSecretExternal(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	revisionID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.secretsBackendProvider.EXPECT().Type().Return("active-type").AnyTimes()
	s.secretsBackendProvider.EXPECT().NewBackend(ptr(backendConfigs.Configs["backend-id"])).DoAndReturn(func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		return s.secretsBackend, nil
	})
	s.secretsBackendProvider.EXPECT().CleanupSecrets(gomock.Any(), ptr(backendConfigs.Configs["backend-id"]), "mariadb/0", provider.SecretRevisions{
		uri.ID: set.NewStrings("rev-id"),
	})

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().ListExternalSecretRevisions(gomock.Any(), uri, 666).Return([]coresecrets.ValueRef{{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}}, nil)
	s.state.EXPECT().DeleteSecret(gomock.Any(), uri, []int{666}).Return([]uuid.UUID{revisionID}, nil)
	s.secretBackendReferenceMutator.EXPECT().RemoveSecretBackendReference(gomock.Any(), revisionID).Return(nil)

	err = s.service.DeleteSecret(context.Background(), uri, DeleteSecretParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{666},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteObsoleteUserSecretRevisions(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	revisionID1, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	revisionID2, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().DeleteObsoleteUserSecretRevisions(gomock.Any()).Return([]uuid.UUID{revisionID1, revisionID2}, nil)
	s.secretBackendReferenceMutator.EXPECT().RemoveSecretBackendReference(gomock.Any(), revisionID1, revisionID2).Return(nil)

	err = s.service.DeleteObsoleteUserSecretRevisions(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}
