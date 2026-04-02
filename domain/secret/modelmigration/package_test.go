// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"time"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secret"
	"github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination migrations_mock_test.go github.com/juju/juju/domain/secret/modelmigration Coordinator,ImportService,SecretBackendService

// backendSecrets provides some secrets to export.
func backendSecrets(uri1, uri2, uri3, uri4 *secrets.URI, nextRotate, expire, timestamp time.Time) *service.SecretImport {
	return &service.SecretImport{
		Secrets: []*secrets.SecretMetadata{{
			URI: uri1,
			Owner: secrets.Owner{
				Kind: secrets.UnitOwner,
				ID:   "mysql/0",
			},
			Description:            "mine",
			Label:                  "ownerlabel",
			LatestRevisionChecksum: "deadbeef",
			RotatePolicy:           secrets.RotateHourly,
			NextRotateTime:         new(nextRotate),
			CreateTime:             timestamp,
			UpdateTime:             timestamp,
		}, {
			URI: uri2,
			Owner: secrets.Owner{
				Kind: secrets.ModelOwner,
				ID:   testing.ModelTag.Id(),
			},
			AutoPrune:              true,
			LatestRevisionChecksum: "deadbeef2",
			CreateTime:             timestamp,
			UpdateTime:             timestamp,
		}, {
			URI: uri3,
			Owner: secrets.Owner{
				Kind: secrets.ApplicationOwner,
				ID:   "mysql",
			},
			CreateTime: timestamp,
			UpdateTime: timestamp,
		}},
		Revisions: map[string][]*secrets.SecretRevisionMetadata{
			uri1.ID: {{
				Revision: 1,
				ValueRef: &secrets.ValueRef{
					BackendID:  "backend-id",
					RevisionID: "revision-id",
				},
				ExpireTime: new(expire),
				CreateTime: timestamp,
				UpdateTime: timestamp,
			}, {
				Revision:   2,
				CreateTime: timestamp,
				UpdateTime: timestamp,
			}},
			uri2.ID: {{
				Revision:   1,
				CreateTime: timestamp,
				UpdateTime: timestamp,
				ValueRef: &secrets.ValueRef{
					BackendID:  "caas-backend",
					RevisionID: "revision-id",
				},
			}},
			uri3.ID: {},
		},
		Content: map[string]map[int]secrets.SecretData{
			uri1.ID: {
				2: map[string]string{
					"foo": "bar",
				},
			},
			uri2.ID: {},
			uri3.ID: {},
		},
		Consumers: map[string][]service.ConsumerInfo{
			uri1.ID: {{
				SecretConsumerMetadata: secrets.SecretConsumerMetadata{
					Label:           "mysecret",
					CurrentRevision: 2,
				},
				Accessor: secret.SecretAccessor{
					Kind: "unit",
					ID:   "mariadb/0",
				},
			}},
			uri2.ID: {{
				SecretConsumerMetadata: secrets.SecretConsumerMetadata{
					Label:           "",
					CurrentRevision: 1,
				},
				Accessor: secret.SecretAccessor{
					Kind: "unit",
					ID:   "mariadb/0",
				},
			}},
			uri3.ID: {},
		},
		Access: map[string][]service.SecretAccess{
			uri1.ID: {{
				Scope: secret.SecretAccessScope{
					Kind: "relation",
					ID:   "mysql:server mariadb:db",
				},
				Subject: secret.SecretAccessor{
					Kind: "application",
					ID:   "mariadb",
				},
				Role: "view",
			}},
			uri2.ID: {{
				Scope: secret.SecretAccessScope{
					Kind: "model",
					ID:   testing.ModelTag.Id(),
				},
				Subject: secret.SecretAccessor{
					Kind: "model",
					ID:   testing.ModelTag.Id(),
				},
				Role: "manage",
			}},
			uri3.ID: nil,
		},
	}
}
