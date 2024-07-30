// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/testing"
)

func (s *serviceSuite) TestGetSecretsForExport(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	secrets := []*coresecrets.SecretMetadata{{
		URI: uri,
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

	s.state = NewMockState(ctrl)
	s.state.EXPECT().ListSecrets(gomock.Any(), nil, nil, domainsecret.NilLabels).Return(
		secrets, revisions, nil,
	)
	s.state.EXPECT().GetSecretValue(gomock.Any(), uri, 1).Return(
		coresecrets.SecretData{"foo": "bar"}, nil, nil,
	)
	s.state.EXPECT().GetSecretValue(gomock.Any(), uri, 3).Return(
		coresecrets.SecretData{"foo": "bar3"}, nil, nil,
	)
	s.state.EXPECT().AllSecretGrants(gomock.Any()).Return(
		map[string][]domainsecret.GrantParams{
			uri.ID: {{
				ScopeTypeID:   1,
				ScopeID:       "wordpress",
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

	got, err := s.service(c).GetSecretsForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, &SecretExport{
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
				Accessor: SecretAccessor{
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
				Accessor: SecretAccessor{
					Kind: "unit",
					ID:   "remote-app/0",
				},
			}},
		},
		Access: map[string][]SecretAccess{
			uri.ID: {{
				Scope: SecretAccessScope{
					Kind: "application",
					ID:   "wordpress",
				},
				Subject: SecretAccessor{
					Kind: "application",
					ID:   "wordpress",
				},
				Role: "manage",
			}},
		},
		RemoteSecrets: nil,
	})
}

func (s *serviceSuite) TestImportSecrets(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

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
		LatestRevisionChecksum: "",
		LatestExpireTime:       ptr(expireTime),
		NextRotateTime:         ptr(rotateTime),
	}, {
		URI:     uri3,
		Version: 0,
		Owner: coresecrets.Owner{
			Kind: coresecrets.ModelOwner,
			ID:   testing.ModelTag.Id(),
		},
		Description:            "a secret",
		LatestRevisionChecksum: "",
		AutoPrune:              true,
	}}
	revisions := [][]*coresecrets.SecretRevisionMetadata{{{
		Revision: 1,
	}, {
		Revision: 2,
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "revision-id",
		},
	}}, {{
		Revision: 5,
	}}}

	s.state = NewMockState(ctrl)

	s.state.EXPECT().UpdateRemoteSecretRevision(gomock.Any(), uri2, 668)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri2, "mysql/0", &coresecrets.SecretConsumerMetadata{
		Label:           "remote label",
		CurrentRevision: 666,
	})

	s.state.EXPECT().CreateCharmApplicationSecret(gomock.Any(), 0, uri, "mysql", domainsecret.UpsertSecretParams{
		RotatePolicy:   ptr(domainsecret.RotateHourly),
		ExpireTime:     nil,
		NextRotateTime: ptr(rotateTime),
		Description:    ptr(secrets[0].Description),
		Label:          ptr(secrets[0].Label),
		AutoPrune:      nil,
		Data:           map[string]string{"foo": "bar"},
	})
	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, domainsecret.UpsertSecretParams{
		ExpireTime: ptr(expireTime),
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "revision-id",
		},
	})
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, "mysql/0", &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	})
	s.state.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0", &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 668,
	})
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   3,
		ScopeID:       "wordpress:db mysql:server",
		SubjectTypeID: 0,
		SubjectID:     "wordpress/0",
		RoleID:        1,
	})

	s.state.EXPECT().GrantAccess(gomock.Any(), uri3, domainsecret.GrantParams{
		ScopeTypeID:   1,
		ScopeID:       "mysql",
		SubjectTypeID: 1,
		SubjectID:     "mysql",
		RoleID:        1,
	})

	s.state.EXPECT().CreateUserSecret(gomock.Any(), 0, uri3, domainsecret.UpsertSecretParams{
		Description: ptr(secrets[1].Description),
		AutoPrune:   ptr(secrets[1].AutoPrune),
		Data:        map[string]string{"foo": "baz"},
	})

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
				Accessor: SecretAccessor{
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
				Accessor: SecretAccessor{
					Kind: "unit",
					ID:   "remote-app/0",
				},
			}},
		},
		Access: map[string][]SecretAccess{
			uri.ID: {{
				Scope: SecretAccessScope{
					Kind: "relation",
					ID:   "wordpress:db mysql:server",
				},
				Subject: SecretAccessor{
					Kind: "unit",
					ID:   "wordpress/0",
				},
				Role: "view",
			}},
			uri3.ID: {{
				Scope: SecretAccessScope{
					Kind: "application",
					ID:   "mysql",
				},
				Subject: SecretAccessor{
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
			Accessor: SecretAccessor{
				Kind: "unit",
				ID:   "mysql/0",
			},
		}},
	}
	err := s.service(c).ImportSecrets(context.Background(), toImport)
	c.Assert(err, jc.ErrorIsNil)
}
