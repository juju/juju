// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/juju/description/v10"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secret/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

func TestExportSuite(t *stdtesting.T) {
	tc.Run(t, &exportSuite{})
}

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation(c *tc.C) *exportOperation {
	return &exportOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}
}

func (s *exportSuite) TestRegisterExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterExport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func ptr[T any](v T) *T {
	return &v
}

// backendSecrets provides some secrets to export.
func backendSecrets(uri1, uri2, uri3, uri4 *secrets.URI, nextRotate, expire, timestamp time.Time) *service.SecretExport {
	return &service.SecretExport{
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
			NextRotateTime:         ptr(nextRotate),
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
				ExpireTime: ptr(expire),
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
			}},
			uri3.ID: {},
		},
		Content: map[string]map[int]secrets.SecretData{
			uri1.ID: {
				2: map[string]string{
					"foo": "bar",
				},
			},
			uri2.ID: {
				1: map[string]string{
					"foo": "bar2",
				},
			},
			uri3.ID: {},
		},
		Consumers: map[string][]service.ConsumerInfo{
			uri1.ID: {{
				SecretConsumerMetadata: secrets.SecretConsumerMetadata{
					Label:           "mysecret",
					CurrentRevision: 2,
				},
				Accessor: service.SecretAccessor{
					Kind: "unit",
					ID:   "mariadb/0",
				},
			}},
			uri2.ID: {{
				SecretConsumerMetadata: secrets.SecretConsumerMetadata{
					Label:           "",
					CurrentRevision: 1,
				},
				Accessor: service.SecretAccessor{
					Kind: "unit",
					ID:   "mariadb/0",
				},
			}},
			uri3.ID: {},
		},
		RemoteConsumers: map[string][]service.ConsumerInfo{
			uri1.ID: {{
				SecretConsumerMetadata: secrets.SecretConsumerMetadata{
					CurrentRevision: 1,
				},
				Accessor: service.SecretAccessor{
					Kind: "unit",
					ID:   "remote-deadbeef/0",
				},
			}},
			uri2.ID: {},
			uri3.ID: {},
		},
		Access: map[string][]service.SecretAccess{
			uri1.ID: {{
				Scope: service.SecretAccessScope{
					Kind: "relation",
					ID:   "mysql:server mariadb:db",
				},
				Subject: service.SecretAccessor{
					Kind: "application",
					ID:   "mariadb",
				},
				Role: "view",
			}, {
				Scope: service.SecretAccessScope{
					Kind: "relation",
					ID:   "mysql:server remote-deadbeef:db",
				},
				Subject: service.SecretAccessor{
					Kind: "remote-application",
					ID:   "remote-deadbeef",
				},
				Role: "view",
			}},
			uri2.ID: {{
				Scope: service.SecretAccessScope{
					Kind: "model",
					ID:   testing.ModelTag.Id(),
				},
				Subject: service.SecretAccessor{
					Kind: "model",
					ID:   testing.ModelTag.Id(),
				},
				Role: "manage",
			}},
			uri3.ID: nil,
		},
		RemoteSecrets: []service.RemoteSecret{{
			URI:             uri4,
			Label:           "mylabel",
			CurrentRevision: 1,
			LatestRevision:  666,
			Accessor: service.SecretAccessor{
				Kind: "unit",
				ID:   "mariadb/0",
			},
		}},
	}
}

// serialisedSecrets represents the expected outcome of exporting secrets.
func serialisedSecrets(uri1, uri2, uri3 *secrets.URI, nextRotate, expire, timestamp time.Time) string {
	return fmt.Sprintf(`
- id: %[1]s
  secret-version: 0
  description: mine
  label: ownerlabel
  rotate-policy: hourly
  owner: unit-mysql-0
  create-time: %[2]s
  update-time: %[2]s
  revisions:
  - number: 1
    create-time: %[2]s
    update-time: %[2]s
    value-ref:
      backend-id: backend-id
      revision-id: revision-id
    expire-time: %[4]s
  - number: 2
    create-time: %[2]s
    update-time: %[2]s
    content:
      foo: bar
  acl:
    application-mariadb:
      scope: relation-mysql.server#mariadb.db
      role: view
    application-remote-deadbeef:
      scope: relation-mysql.server#remote-deadbeef.db
      role: view
  consumers:
  - consumer: unit-mariadb-0
    label: mysecret
    current-revision: 2
  remote-consumers:
  - id: ""
    consumer: unit-remote-deadbeef-0
    current-revision: 1
  next-rotate-time: %[3]s
  latest-revision-checksum: deadbeef
- id: %[5]s
  secret-version: 0
  description: ""
  label: ""
  rotate-policy: never
  owner: model-deadbeef-0bad-400d-8000-4b1d0d06f00d
  auto-prune: true
  create-time: %[2]s
  update-time: %[2]s
  revisions:
  - number: 1
    create-time: %[2]s
    update-time: %[2]s
    content:
      foo: bar2
  acl:
    model-deadbeef-0bad-400d-8000-4b1d0d06f00d:
      scope: model-deadbeef-0bad-400d-8000-4b1d0d06f00d
      role: manage
  consumers:
  - consumer: unit-mariadb-0
    label: ""
    current-revision: 1
  latest-revision-checksum: deadbeef2
- id: %[6]s
  secret-version: 0
  description: ""
  label: ""
  rotate-policy: never
  owner: application-mysql
  create-time: %[2]s
  update-time: %[2]s
  revisions: []
  latest-revision-checksum: ""
`[1:], uri1.ID,
		timestamp.UTC().Format(time.RFC3339Nano),
		nextRotate.UTC().Format(time.RFC3339Nano),
		expire.UTC().Format(time.RFC3339Nano),
		uri2.ID,
		uri3.ID,
	)
}

func serialisedRemoteSecrets(uri *secrets.URI) string {
	return fmt.Sprintf(`
- id: %s
  source-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  consumer: unit-mariadb-0
  label: mylabel
  current-revision: 1
  latest-revision: 666
`[1:], uri.ID,
	)
}

func (s *exportSuite) TestExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	uri := secrets.NewURI()
	uri2 := secrets.NewURI()
	uri3 := secrets.NewURI()
	uri4 := secrets.NewURI().WithSource(testing.ModelTag.Id())
	nextRotate := time.Now()
	expire := time.Now()
	timestamp := time.Now()

	forExport := backendSecrets(uri, uri2, uri3, uri4, nextRotate, expire, timestamp)
	s.service.EXPECT().GetSecretsForExport(gomock.Any()).
		Return(forExport, nil)

	op := s.newExportOperation(c)
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIsNil)

	actualSecrets := dst.Secrets()
	c.Assert(len(actualSecrets), tc.Equals, 3)
	out, err := yaml.Marshal(actualSecrets)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, serialisedSecrets(uri, uri2, uri3, nextRotate, expire, timestamp))

	actualRemoteSecrets := dst.RemoteSecrets()
	c.Assert(len(actualRemoteSecrets), tc.Equals, 1)
	out, err = yaml.Marshal(actualRemoteSecrets)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, serialisedRemoteSecrets(uri4))
}
