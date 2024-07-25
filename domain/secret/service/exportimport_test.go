// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
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
	}}}

	s.state = NewMockState(ctrl)
	s.state.EXPECT().ListSecrets(gomock.Any(), nil, nil, domainsecret.NilLabels).Return(
		secrets, revisions, nil,
	)
	s.state.EXPECT().GetSecretValue(gomock.Any(), uri, 1).Return(
		coresecrets.SecretData{"foo": "bar"}, nil, nil,
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
