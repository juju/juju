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
)

func (s *serviceSuite) TestDeleteSecretInternal(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().ListExternalSecretRevisions(gomock.Any(), uri, 666).Return([]coresecrets.ValueRef{}, nil)
	s.state.EXPECT().DeleteSecret(gomock.Any(), uri, []int{666}).Return([]string{"rev-uuid-1", "rev-uuid-2"}, nil)
	s.secretBackendReferenceMutator.EXPECT().RemoveSecretBackendReference(gomock.Any(), "rev-uuid-1", "rev-uuid-2").Return(nil)

	err := s.service.DeleteSecret(context.Background(), uri, DeleteSecretParams{
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

	p := NewMockSecretBackendProvider(ctrl)
	p.EXPECT().Type().Return("active-type").AnyTimes()
	p.EXPECT().NewBackend(ptr(backendConfigs.Configs["backend-id"])).DoAndReturn(func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		return s.secretsBackend, nil
	})
	p.EXPECT().CleanupSecrets(gomock.Any(), ptr(backendConfigs.Configs["backend-id"]), "mariadb/0", provider.SecretRevisions{
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
	s.state.EXPECT().DeleteSecret(gomock.Any(), uri, []int{666}).Return(nil)

	err := s.serviceWithProvider(c, p).DeleteSecret(context.Background(), uri, DeleteSecretParams{
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

	s.state.EXPECT().DeleteObsoleteUserSecretRevisions(gomock.Any()).Return([]string{"rev-uuid-1", "rev-uuid-2"}, nil)
	s.secretBackendReferenceMutator.EXPECT().RemoveSecretBackendReference(gomock.Any(), "rev-uuid-1", "rev-uuid-2").Return(nil)

	err := s.service.DeleteObsoleteUserSecretRevisions(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}
