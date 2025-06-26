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

	"github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type importSuite struct {
	coordinator    *MockCoordinator
	service        *MockImportService
	backendService *MockSecretBackendService
}

func TestImportSuite(t *stdtesting.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)
	s.backendService = NewMockSecretBackendService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation(c *tc.C) *importOperation {
	return &importOperation{
		service:        s.service,
		backendService: s.backendService,
		logger:         loggertesting.WrapCheckLog(c),
	}
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c))
}

// serialisedModel provides a model with secrets to import.
func serialisedModel(uri1, uri2, uri3, uri4 *secrets.URI, nextRotate, expire, timestamp time.Time) string {
	return fmt.Sprintf(`
version: 11
agent-version: "6.6.6"
type: "iaas"
owner: ""
config: {}
environ-version: 0
storage-pools:
  pools: []
  version: 1
users:
  version: 1
  users: []
machines:
  version: 3
  machines: []
applications:
  version: 12
  applications: []
relations:
  version: 3
  relations: []
remote-entities:
  version: 1
  remote-entities: []
relation-networks:
  version: 1
  relation-networks: []
offer-connections:
  version: 1
  offer-connections: []
external-controllers:
  version: 1
  external-controllers: []
spaces:
  version: 3
  spaces: []
link-layer-devices:
  version: 1
  link-layer-devices: []
ip-addresses:
  version: 5
  ip-addresses: []
subnets:
  version: 6
  subnets: []
cloud-image-metadata:
  version: 2
  cloudimagemetadata: []
status:
  version: 1
  status:
    updated: 2017-02-21T19:47:23.691434191Z
    value: active
actions:
  version: 4
  actions: []
operations:
  version: 2
  operations: []
ssh-host-keys:
  version: 1
  ssh-host-keys: []
sequences: {}
cloud: ""
volumes:
  version: 1
  volumes: []
filesystems:
  version: 1
  filesystems: []
storages:
  version: 3
  storages: []
storage-pools:
  version: 1
  pools: []
firewall-rules:
  version: 1
  firewall-rules: []
remote-applications:
  version: 3
  remote-applications: []
sla:
  level: ""
  owner: ""
  credentials: ""
meter-status:
  code: ""
  info: ""
secrets:
  version: 2
  secrets:
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
    rotate-policy: ""
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
      current-revision: 0
    latest-revision-checksum: deadbeef2
  - id: %[6]s
    secret-version: 0
    description: ""
    label: ""
    rotate-policy: ""
    owner: application-mysql
    create-time: %[2]s
    update-time: %[2]s
    revisions: []
remote-secrets:
  version: 1
  remote-secrets:
  - id: %[7]s
    source-uuid: %[8]s
    consumer: unit-mariadb-0
    label: mylabel
    current-revision: 1
    latest-revision: 666
`[1:], uri1.ID,
		timestamp.UTC().Format(time.RFC3339Nano),
		nextRotate.UTC().Format(time.RFC3339Nano),
		expire.UTC().Format(time.RFC3339Nano),
		uri2.ID,
		uri3.ID,
		uri4.ID,
		uri4.SourceUUID,
	)
}

func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := secrets.NewURI()
	uri2 := secrets.NewURI()
	uri3 := secrets.NewURI()
	uri4 := secrets.NewURI().WithSource(testing.ModelTag.Id())
	nextRotate := time.Now().UTC()
	expire := time.Now().UTC()
	timestamp := time.Now().UTC()

	model := serialisedModel(uri, uri2, uri3, uri4, nextRotate, expire, timestamp)

	dst, err := description.Deserialize([]byte(model))
	c.Assert(err, tc.ErrorIsNil)

	s.backendService.EXPECT().ListBackendIDs(gomock.Any()).Return([]string{"backend-id"}, nil)
	forImport := backendSecrets(uri, uri2, uri3, uri4, nextRotate, expire, timestamp)
	s.service.EXPECT().ImportSecrets(gomock.Any(), forImport)

	op := s.newImportOperation(c)
	err = op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportMissingBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := secrets.NewURI()
	uri2 := secrets.NewURI()
	uri3 := secrets.NewURI()
	uri4 := secrets.NewURI().WithSource(testing.ModelTag.Id())
	nextRotate := time.Now().UTC()
	expire := time.Now().UTC()
	timestamp := time.Now().UTC()

	model := serialisedModel(uri, uri2, uri3, uri4, nextRotate, expire, timestamp)

	dst, err := description.Deserialize([]byte(model))
	c.Assert(err, tc.ErrorIsNil)

	s.backendService.EXPECT().ListBackendIDs(gomock.Any()).Return([]string{"backend-id2"}, nil)

	op := s.newImportOperation(c)
	err = op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIs, secreterrors.MissingSecretBackendID)
}
