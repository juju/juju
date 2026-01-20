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
	domainsecret "github.com/juju/juju/domain/secret"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testing"
)

func (s *serviceSuite) TestGetSecretsForExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	secrets := []*coresecrets.SecretMetadata{{
		URI:                    uri,
		LatestRevisionChecksum: "checksum-1234",
	}}
	revisions := [][]*coresecrets.SecretRevisionMetadata{{{
		Revision: 1,
	}, {
		Revision: 2,
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "revision-id",
		},
	}, {
		Revision: 3,
	}}}

	s.state.EXPECT().ListAllSecrets(gomock.Any()).Return(
		secrets, revisions, nil,
	)
	s.state.EXPECT().GetSecretValue(gomock.Any(), uri, 1).Return(
		coresecrets.SecretData{"foo": "bar"}, nil, nil,
	)
	s.state.EXPECT().GetSecretValue(gomock.Any(), uri, 3).Return(
		coresecrets.SecretData{"foo": "bar3"}, nil, nil,
	)
	s.state.EXPECT().AllSecretGrants(gomock.Any()).Return(
		map[string][]domainsecret.GrantDetails{
			uri.ID: {{
				ScopeTypeID:   1,
				ScopeID:       "wordpress",
				ScopeUUID:     "wordpress-uuid",
				SubjectTypeID: 1,
				SubjectID:     "wordpress",
				RoleID:        2,
			}},
		}, nil,
	)
	s.state.EXPECT().AllSecretConsumers(gomock.Any()).Return(
		map[string][]domainsecret.ConsumerInfo{
			uri.ID: {{
				SubjectTypeID:   0,
				SubjectID:       "mysql/0",
				Label:           "my label",
				CurrentRevision: 666,
			}},
		}, nil,
	)
	s.state.EXPECT().AllSecretRemoteConsumers(gomock.Any()).Return(
		map[string][]domainsecret.ConsumerInfo{
			uri.ID: {{
				SubjectTypeID:   0,
				SubjectID:       "remote-app/0",
				CurrentRevision: 668,
			}},
		}, nil,
	)
	s.state.EXPECT().AllRemoteSecrets(gomock.Any()).Return(
		[]domainsecret.RemoteSecretInfo{}, nil,
	)

	got, err := s.service.GetSecretsForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, &SecretExport{
		Secrets: secrets,
		Revisions: map[string][]*coresecrets.SecretRevisionMetadata{
			uri.ID: revisions[0],
		},
		Content: map[string]map[int]coresecrets.SecretData{
			uri.ID: {
				1: {"foo": "bar"},
				3: {"foo": "bar3"},
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
		RemoteConsumers: map[string][]ConsumerInfo{
			uri.ID: {{
				SecretConsumerMetadata: coresecrets.SecretConsumerMetadata{
					CurrentRevision: 668,
				},
				Accessor: domainsecret.SecretAccessor{
					Kind: "unit",
					ID:   "remote-app/0",
				},
			}},
		},
		Access: map[string][]SecretAccess{
			uri.ID: {{
				Scope: domainsecret.SecretAccessScope{
					Kind: "application",
					ID:   "wordpress",
				},
				Subject: domainsecret.SecretAccessor{
					Kind: "application",
					ID:   "wordpress",
				},
				Role: "manage",
			}},
		},
		RemoteSecrets: nil,
	})
}

func (s *serviceSuite) TestImportSecrets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC().Truncate(time.Hour * 24)

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	uri3 := coresecrets.NewURI()
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
		LatestExpireTime:       ptr(expireTime),
		NextRotateTime:         ptr(rotateTime),
		CreateTime:             now.Add(1 * time.Hour),
		UpdateTime:             now.Add(3 * time.Hour),
	}, {
		URI:     uri3,
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
			},
			{
				Revision:   2,
				CreateTime: now.Add(3 * time.Hour),
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

	// TODO(secrets) - move to crossmodelrelation domain
	// s.state.EXPECT().UpdateRemoteSecretRevision(gomock.Any(), uri2, 668)
	//s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri2, unittesting.GenNewName(c, "mysql/0"), coresecrets.SecretConsumerMetadata{
	//	Label:           "remote label",
	//	CurrentRevision: 666,
	//})

	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetApplicationUUID(domaintesting.IsAtomicContextChecker, "mysql").Return(appUUID, nil).AnyTimes()
	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUID(domaintesting.IsAtomicContextChecker, coreunit.Name("wordpress/0")).Return(unitUUID, nil)
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

	s.state.EXPECT().ImportSecretWithRevision(gomock.Any(), 0, uri, domainsecret.Owner{
		Kind: coresecrets.ApplicationOwner,
		UUID: appUUID.String(),
	}, domainsecret.UpsertSecretParams{
		CreateTime:     secrets[0].CreateTime,
		UpdateTime:     secrets[0].UpdateTime,
		NextRotateTime: ptr(rotateTime),
		Description:    ptr(secrets[0].Description),
		Label:          ptr(secrets[0].Label),
		RotatePolicy:   ptr(domainsecret.RotateHourly),
		Checksum:       "checksum-1234",
		ExpireTime:     ptr(expireTime),
	}, []domainsecret.ImportRevision{
		{
			Revision: 1,
			Params: domainsecret.UpsertSecretParams{
				CreateTime: revisions[0][0].CreateTime,
				UpdateTime: revisions[0][0].CreateTime,
				RevisionID: ptr(s.fakeUUID.String()),
				Data:       map[string]string{"foo": "bar"},
			},
		},
		{
			Revision: 2,
			Params: domainsecret.UpsertSecretParams{
				CreateTime: revisions[0][1].CreateTime,
				UpdateTime: revisions[0][1].CreateTime,
				RevisionID: ptr(s.fakeUUID.String()),
				ValueRef: &coresecrets.ValueRef{
					BackendID:  "backend-id",
					RevisionID: "revision-id",
				},
				Checksum:   "checksum-1234",
				ExpireTime: ptr(expireTime),
			},
		},
	}).Return(nil)

	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(
		func() error { return nil }, nil,
	)
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "revision-id",
	}, s.modelID, s.fakeUUID.String()).Return(
		func() error { return nil }, nil,
	)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mysql/0"), coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	})
	// TODO(secrets) - move to crossmodelrelation domain
	//s.state.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "remote-app/0"), coresecrets.SecretConsumerMetadata{
	//	CurrentRevision: 668,
	//})
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   3,
		ScopeUUID:     relUUID.String(),
		SubjectTypeID: 0,
		SubjectUUID:   unitUUID.String(),
		RoleID:        1,
	})

	s.state.EXPECT().GrantAccess(gomock.Any(), uri3, domainsecret.GrantParams{
		ScopeTypeID:   1,
		ScopeUUID:     appUUID.String(),
		SubjectTypeID: 1,
		SubjectUUID:   appUUID.String(),
		RoleID:        1,
	})

	s.state.EXPECT().ImportSecretWithRevision(gomock.Any(), 0, uri3, domainsecret.Owner{
		Kind: coresecrets.ModelOwner,
		UUID: testing.ModelTag.Id(),
	}, domainsecret.UpsertSecretParams{
		CreateTime:  secrets[1].CreateTime,
		UpdateTime:  secrets[1].UpdateTime,
		Description: ptr(secrets[1].Description),
		AutoPrune:   ptr(secrets[1].AutoPrune),
		Checksum:    "checksum-1234",
	}, []domainsecret.ImportRevision{
		{
			Revision: 5,
			Params: domainsecret.UpsertSecretParams{
				CreateTime: revisions[1][0].CreateTime,
				UpdateTime: revisions[1][0].CreateTime,
				RevisionID: ptr(s.fakeUUID.String()),
				Data:       map[string]string{"foo": "baz"},
				Checksum:   "checksum-1234",
			},
		},
	}).Return(nil)
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(
		func() error { return nil }, nil,
	)

	toImport := &SecretExport{
		Secrets: secrets,
		Revisions: map[string][]*coresecrets.SecretRevisionMetadata{
			uri.ID:  revisions[0],
			uri3.ID: revisions[1],
		},
		Content: map[string]map[int]coresecrets.SecretData{
			uri.ID: {
				1: {"foo": "bar"},
			},
			uri3.ID: {
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
		RemoteConsumers: map[string][]ConsumerInfo{
			uri.ID: {{
				SecretConsumerMetadata: coresecrets.SecretConsumerMetadata{
					CurrentRevision: 668,
				},
				Accessor: domainsecret.SecretAccessor{
					Kind: "unit",
					ID:   "remote-app/0",
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
			uri3.ID: {{
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
		RemoteSecrets: []RemoteSecret{{
			URI:             uri2,
			Label:           "remote label",
			CurrentRevision: 666,
			LatestRevision:  668,
			Accessor: domainsecret.SecretAccessor{
				Kind: "unit",
				ID:   "mysql/0",
			},
		}},
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
			},
			{
				Revision:   2,
				CreateTime: now.Add(3 * time.Hour),
			},
		},
	}

	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetApplicationUUID(domaintesting.IsAtomicContextChecker, "mysql").Return(appUUID, nil).AnyTimes()
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)

	// First revision succeeds.
	rolledBack1 := false
	rollback1 := func() error {
		rolledBack1 = true
		return nil
	}
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), gomock.Any(), s.modelID, s.fakeUUID.String()).Return(
		rollback1, nil,
	)

	// Second revision fails.
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), gomock.Any(), s.modelID, s.fakeUUID.String()).Return(
		nil, fmt.Errorf("failed to add reference"),
	)

	toImport := &SecretExport{
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
	s.state.EXPECT().GetApplicationUUID(domaintesting.IsAtomicContextChecker, "mysql").Return(appUUID, nil).AnyTimes()
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)

	// Backend reference succeeds.
	rolledBack := false
	rollback := func() error {
		rolledBack = true
		return nil
	}
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), gomock.Any(), s.modelID, s.fakeUUID.String()).Return(
		rollback, nil,
	)

	// State import fails.
	s.state.EXPECT().ImportSecretWithRevision(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
		fmt.Errorf("failed to import to state"),
	)

	toImport := &SecretExport{
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
