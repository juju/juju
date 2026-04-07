// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/relation"
	relationtesting "github.com/juju/juju/core/relation/testing"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/deployment/charm"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/testing"
)

func (s *serviceSuite) TestImportSecrets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC().Truncate(time.Hour * 24)

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	expireTime := time.Now()
	rotateTime := time.Now()
	secrets := []*coresecrets.SecretMetadata{{
		URI:     uri,
		Version: 0,
		Owner: coresecrets.Owner{
			Kind: coresecrets.ApplicationOwner,
			ID:   "mysql",
		},
		Description:            "my secret",
		Label:                  "a secret",
		RotatePolicy:           "hourly",
		LatestRevisionChecksum: "checksum-1234",
		LatestExpireTime:       new(expireTime),
		NextRotateTime:         new(rotateTime),
		CreateTime:             now.Add(1 * time.Hour),
		UpdateTime:             now.Add(3 * time.Hour),
	}, {
		URI:     uri2,
		Version: 0,
		Owner: coresecrets.Owner{
			Kind: coresecrets.ModelOwner,
			ID:   testing.ModelTag.Id(),
		},
		Description:            "a secret",
		LatestRevisionChecksum: "checksum-1234",
		AutoPrune:              true,
		CreateTime:             now.Add(4 * time.Hour),
		UpdateTime:             now.Add(6 * time.Hour),
	}}
	revisions := [][]*coresecrets.SecretRevisionMetadata{
		{
			{
				Revision:   1,
				CreateTime: now.Add(1 * time.Hour),
				UpdateTime: now.Add(10 * time.Hour),
			},
			{
				Revision:   2,
				CreateTime: now.Add(3 * time.Hour),
				UpdateTime: now.Add(13 * time.Hour),
				ExpireTime: new(expireTime),
				ValueRef: &coresecrets.ValueRef{
					BackendID:  "backend-id",
					RevisionID: "revision-id",
				},
			},
		},
		{
			{
				Revision:   5,
				CreateTime: now.Add(4 * time.Hour),
			},
		},
	}

	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetApplicationUUID(c.Context(), "mysql").Return(appUUID, nil).AnyTimes()
	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUID(c.Context(), coreunit.Name("wordpress/0")).Return(unitUUID, nil)
	relUUID := relationtesting.GenRelationUUID(c)
	s.state.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(gomock.Any(), relation.EndpointIdentifier{
		ApplicationName: "wordpress",
		EndpointName:    "db",
		Role:            charm.RoleRequirer,
	}, relation.EndpointIdentifier{
		ApplicationName: "mysql",
		EndpointName:    "server",
		Role:            charm.RoleProvider,
	}).Return(relUUID.String(), nil)

	s.state.EXPECT().ImportSecretWithRevisions(gomock.Any(), 0, uri, domainsecret.Owner{
		Kind: coresecrets.ApplicationOwner,
		UUID: appUUID.String(),
	}, domainsecret.UpsertSecretParams{
		CreateTime:     secrets[0].CreateTime,
		UpdateTime:     secrets[0].UpdateTime,
		NextRotateTime: new(rotateTime),
		Description:    new(secrets[0].Description),
		Label:          new(secrets[0].Label),
		RotatePolicy:   new(domainsecret.RotateHourly),
		Checksum:       "checksum-1234",
		ExpireTime:     new(expireTime),
	}, []domainsecret.UpsertRevisionParams{
		{
			Revision:   1,
			CreateTime: revisions[0][0].CreateTime,
			UpdateTime: revisions[0][0].UpdateTime,
			RevisionID: new(s.fakeUUID.String()),
			Data:       map[string]string{"foo": "bar"},
		},
		{
			Revision:   2,
			CreateTime: revisions[0][1].CreateTime,
			UpdateTime: revisions[0][1].UpdateTime,
			RevisionID: new(s.fakeUUID.String()),
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "revision-id",
			},
			Checksum:   "checksum-1234",
			ExpireTime: new(expireTime),
		},
	}).Return(nil)

	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String(), uri.ID).
		Return(func() error { return nil }, nil)
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "revision-id",
	}, s.modelID, s.fakeUUID.String(), uri.ID).
		Return(func() error { return nil }, nil)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mysql/0"), coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	})
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   3,
		ScopeUUID:     relUUID.String(),
		SubjectTypeID: 0,
		SubjectUUID:   unitUUID.String(),
		RoleID:        1,
	})

	s.state.EXPECT().GrantAccess(gomock.Any(), uri2, domainsecret.GrantParams{
		ScopeTypeID:   1,
		ScopeUUID:     appUUID.String(),
		SubjectTypeID: 1,
		SubjectUUID:   appUUID.String(),
		RoleID:        1,
	})

	s.state.EXPECT().ImportSecretWithRevisions(gomock.Any(), 0, uri2, domainsecret.Owner{
		Kind: coresecrets.ModelOwner,
		UUID: testing.ModelTag.Id(),
	}, domainsecret.UpsertSecretParams{
		CreateTime:  secrets[1].CreateTime,
		UpdateTime:  secrets[1].UpdateTime,
		Description: new(secrets[1].Description),
		AutoPrune:   new(secrets[1].AutoPrune),
		Checksum:    "checksum-1234",
	}, []domainsecret.UpsertRevisionParams{
		{
			Revision:   5,
			CreateTime: revisions[1][0].CreateTime,
			UpdateTime: revisions[1][0].UpdateTime,
			RevisionID: new(s.fakeUUID.String()),
			Data:       map[string]string{"foo": "baz"},
			Checksum:   "checksum-1234",
		},
	}).Return(nil)
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String(), uri2.ID).
		Return(func() error { return nil }, nil)

	toImport := &SecretImport{
		Secrets: secrets,
		Revisions: map[string][]*coresecrets.SecretRevisionMetadata{
			uri.ID:  revisions[0],
			uri2.ID: revisions[1],
		},
		Content: map[string]map[int]coresecrets.SecretData{
			uri.ID: {
				1: {"foo": "bar"},
			},
			uri2.ID: {
				5: {"foo": "baz"},
			},
		},
		Consumers: map[string][]ConsumerInfo{
			uri.ID: {{
				SecretConsumerMetadata: coresecrets.SecretConsumerMetadata{
					Label:           "my label",
					CurrentRevision: 666,
				},
				Accessor: domainsecret.SecretAccessor{
					Kind: "unit",
					ID:   "mysql/0",
				},
			}},
		},
		Access: map[string][]SecretAccess{
			uri.ID: {{
				Scope: domainsecret.SecretAccessScope{
					Kind: "relation",
					ID:   "wordpress:db mysql:server",
				},
				Subject: domainsecret.SecretAccessor{
					Kind: "unit",
					ID:   "wordpress/0",
				},
				Role: "view",
			}},
			uri2.ID: {{
				Scope: domainsecret.SecretAccessScope{
					Kind: "application",
					ID:   "mysql",
				},
				Subject: domainsecret.SecretAccessor{
					Kind: "application",
					ID:   "mysql",
				},
				Role: "view",
			}},
		},
	}
	err := s.service.ImportSecrets(c.Context(), toImport)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestImportSecretsRollbackOnFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC().Truncate(time.Hour * 24)

	uri := coresecrets.NewURI()
	secrets := []*coresecrets.SecretMetadata{{
		URI:     uri,
		Version: 0,
		Owner: coresecrets.Owner{
			Kind: coresecrets.ApplicationOwner,
			ID:   "mysql",
		},
		LatestRevisionChecksum: "checksum-1234",
		CreateTime:             now.Add(1 * time.Hour),
		UpdateTime:             now.Add(3 * time.Hour),
	}}
	revisions := [][]*coresecrets.SecretRevisionMetadata{
		{
			{
				Revision:   1,
				CreateTime: now.Add(1 * time.Hour),
				UpdateTime: now.Add(10 * time.Hour),
			},
			{
				Revision:   2,
				CreateTime: now.Add(3 * time.Hour),
				UpdateTime: now.Add(13 * time.Hour),
			},
		},
	}

	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetApplicationUUID(c.Context(), "mysql").Return(appUUID, nil).AnyTimes()
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)

	// First revision succeeds.
	rolledBack1 := false
	rollback1 := func() error {
		rolledBack1 = true
		return nil
	}
	s.secretBackendState.EXPECT().AddSecretBackendReference(
		gomock.Any(), gomock.Any(), s.modelID, s.fakeUUID.String(), uri.ID,
	).Return(
		rollback1, nil,
	)

	// Second revision fails.
	s.secretBackendState.EXPECT().AddSecretBackendReference(
		gomock.Any(), gomock.Any(), s.modelID, s.fakeUUID.String(), uri.ID,
	).Return(
		nil, fmt.Errorf("failed to add reference"),
	)

	toImport := &SecretImport{
		Secrets: secrets,
		Revisions: map[string][]*coresecrets.SecretRevisionMetadata{
			uri.ID: revisions[0],
		},
		Content: map[string]map[int]coresecrets.SecretData{
			uri.ID: {
				1: {"foo": "bar"},
				2: {"foo": "baz"},
			},
		},
	}
	err := s.service.ImportSecrets(c.Context(), toImport)
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.ErrorMatches, ".*failed to add reference.*")

	// Verify that the rollback for the first revision was called.
	c.Check(rolledBack1, tc.IsTrue)
}

func (s *serviceSuite) TestImportSecretsRollbackOnStateFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC().Truncate(time.Hour * 24)

	uri := coresecrets.NewURI()
	secrets := []*coresecrets.SecretMetadata{{
		URI:     uri,
		Version: 0,
		Owner: coresecrets.Owner{
			Kind: coresecrets.ApplicationOwner,
			ID:   "mysql",
		},
		LatestRevisionChecksum: "checksum-1234",
		CreateTime:             now.Add(1 * time.Hour),
		UpdateTime:             now.Add(3 * time.Hour),
	}}
	revisions := [][]*coresecrets.SecretRevisionMetadata{
		{
			{
				Revision:   1,
				CreateTime: now.Add(1 * time.Hour),
			},
		},
	}

	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetApplicationUUID(c.Context(), "mysql").Return(appUUID, nil).AnyTimes()
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)

	// Backend reference succeeds.
	rolledBack := false
	rollback := func() error {
		rolledBack = true
		return nil
	}
	s.secretBackendState.EXPECT().AddSecretBackendReference(
		gomock.Any(), gomock.Any(), s.modelID, s.fakeUUID.String(), uri.ID,
	).Return(
		rollback, nil,
	)

	// State import fails.
	s.state.EXPECT().ImportSecretWithRevisions(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return(
		fmt.Errorf("failed to import to state"),
	)

	toImport := &SecretImport{
		Secrets: secrets,
		Revisions: map[string][]*coresecrets.SecretRevisionMetadata{
			uri.ID: revisions[0],
		},
		Content: map[string]map[int]coresecrets.SecretData{
			uri.ID: {
				1: {"foo": "bar"},
			},
		},
	}
	err := s.service.ImportSecrets(c.Context(), toImport)
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.ErrorMatches, ".*failed to import to state.*")

	// Verify that the rollback was called.
	c.Check(rolledBack, tc.IsTrue)
}
