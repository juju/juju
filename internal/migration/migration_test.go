// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"strings"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/core/semversion"
	corestorage "github.com/juju/juju/core/storage"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/state"
)

type ExportSuite struct {
	storageRegistryGetter *MockModelStorageRegistryGetter
	operationsExporter    *MockOperationExporter
	coordinator           *MockCoordinator
	model                 *MockModel
}

func TestExportSuite(t *stdtesting.T) {
	tc.Run(t, &ExportSuite{})
}

func (s *ExportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.storageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)
	s.operationsExporter = NewMockOperationExporter(ctrl)
	s.coordinator = NewMockCoordinator(ctrl)
	s.model = NewMockModel(ctrl)

	return ctrl
}

func (s *ExportSuite) TestExportValidates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	scope := modelmigration.NewScope(nil, nil, nil)

	// The order of the expectations is important here. We expect that the
	// validation is the last thing that happens.
	gomock.InOrder(
		s.operationsExporter.EXPECT().ExportOperations(s.storageRegistryGetter),
		s.coordinator.EXPECT().Perform(gomock.Any(), scope, s.model).Return(nil),
		s.model.EXPECT().Validate().Return(nil),
	)

	exporter := migration.NewModelExporter(
		s.operationsExporter,
		nil,
		scope,
		s.storageRegistryGetter,
		s.coordinator,
		nil, nil,
	)

	_, err := exporter.Export(c.Context(), s.model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ExportSuite) TestExportValidationFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	scope := modelmigration.NewScope(nil, nil, nil)

	s.operationsExporter.EXPECT().ExportOperations(s.storageRegistryGetter)
	s.model.EXPECT().Validate().Return(errors.New("boom"))
	s.coordinator.EXPECT().Perform(gomock.Any(), scope, s.model).Return(nil)

	exporter := migration.NewModelExporter(
		s.operationsExporter,
		nil,
		scope,
		s.storageRegistryGetter,
		s.coordinator,
		nil, nil,
	)

	_, err := exporter.Export(c.Context(), s.model)
	c.Assert(err, tc.ErrorMatches, "boom")
}

type ImportSuite struct {
	testhelpers.IsolationSuite
	charmService     *MockCharmService
	agentBinaryStore *MockAgentBinaryStore
}

func TestImportSuite(t *stdtesting.T) {
	tc.Run(t, &ImportSuite{})
}

func (s *ImportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.charmService = NewMockCharmService(ctrl)
	s.agentBinaryStore = NewMockAgentBinaryStore(ctrl)

	return ctrl
}

func (s *ImportSuite) TestBadBytes(c *tc.C) {
	bytes := []byte("not a model")
	scope := func(model.UUID) modelmigration.Scope { return modelmigration.NewScope(nil, nil, nil) }
	controller := &fakeImporter{}
	importer := migration.NewModelImporter(
		controller, scope, nil, nil,
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return provider.CommonStorageProviders()
		}),
		nil,
		loggertesting.WrapCheckLog(c),
		clock.WallClock,
	)
	model, st, err := importer.ImportModel(c.Context(), bytes)
	c.Check(st, tc.IsNil)
	c.Check(model, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "yaml: unmarshal errors:\n.*")
}

const modelYaml = `
cloud: dev
config:
  name: foo
  type: lxd
  uuid: bd3fae18-5ea1-4bc5-8837-45400cf1f8f6
actions:
  actions: []
  version: 1
applications:
  applications: []
  version: 1
cloud-image-metadata:
  cloudimagemetadata: []
  version: 1
filesystems:
  filesystems: []
  version: 1
ip-addresses:
  ip-addresses: []
  version: 1
link-layer-devices:
  link-layer-devices: []
  version: 1
machines:
  machines: []
  version: 1
owner: admin
relations:
  relations: []
  version: 1
sequences:
  machine: 2
spaces:
  spaces: []
  version: 1
ssh-host-keys:
  ssh-host-keys: []
  version: 1
storage-pools:
  pools: []
  version: 1
storages:
  storages: []
  version: 1
subnets:
  subnets: []
  version: 1
users:
  users: []
  version: 1
volumes:
  volumes: []
  version: 1
version: 1
`

func (s *ImportSuite) TestUploadBinariesConfigValidate(c *tc.C) {
	type T migration.UploadBinariesConfig // alias for brevity

	check := func(modify func(*T), missing string) {
		config := T{
			CharmService:       struct{ migration.CharmService }{},
			CharmUploader:      struct{ migration.CharmUploader }{},
			AgentBinaryStore:   struct{ migration.AgentBinaryStore }{},
			ToolsUploader:      struct{ migration.ToolsUploader }{},
			ResourceDownloader: struct{ migration.ResourceDownloader }{},
			ResourceUploader:   struct{ migration.ResourceUploader }{},
		}
		modify(&config)
		realConfig := migration.UploadBinariesConfig(config)
		c.Check(realConfig.Validate(), tc.ErrorMatches, fmt.Sprintf("missing %s not valid", missing))
	}

	check(func(c *T) { c.CharmService = nil }, "CharmService")
	check(func(c *T) { c.CharmUploader = nil }, "CharmUploader")
	check(func(c *T) { c.AgentBinaryStore = nil }, "AgentBinaryStore")
	check(func(c *T) { c.ToolsUploader = nil }, "ToolsUploader")
	check(func(c *T) { c.ResourceDownloader = nil }, "ResourceDownloader")
	check(func(c *T) { c.ResourceUploader = nil }, "ResourceUploader")
}

func (s *ImportSuite) TestBinariesMigration(c *tc.C) {
	defer s.setupMocks(c).Finish()

	downloader := &fakeDownloader{}
	uploader := &fakeUploader{
		tools:     make(map[semversion.Binary]string),
		resources: make(map[string]string),
	}

	toolsMap := map[string]semversion.Binary{
		"439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e": semversion.MustParseBinary("2.1.0-ubuntu-amd64"),
		"c4e12eaa8a3bf7a1a3029e2cbfccb2d88f59e8efc19f2531c423ce515afcb436": semversion.MustParseBinary("2.0.0-ubuntu-amd64"),
	}

	dataStream := ioutil.NopCloser(strings.NewReader("test agent data"))

	for sha := range toolsMap {
		s.agentBinaryStore.EXPECT().GetAgentBinaryForSHA256(gomock.Any(), sha).Return(dataStream, 15, nil)
	}

	app0Res := resourcetesting.NewResource(c, nil, "blob0", "app0", "blob0").Resource
	app1Res := resourcetesting.NewResource(c, nil, "blob1", "app1", "blob1").Resource
	app2Res := resourcetesting.NewPlaceholderResource(c, "blob2", "app2")
	resources := []resource.Resource{app0Res, app1Res, app2Res}

	s.charmService.EXPECT().GetCharmArchive(gomock.Any(), domaincharm.CharmLocator{
		Name:     "postgresql",
		Revision: 42,
		Source:   domaincharm.CharmHubSource,
	}).Return(ioutil.NopCloser(strings.NewReader("postgresql content")), "hash0123", nil)
	s.charmService.EXPECT().GetCharmArchive(gomock.Any(), domaincharm.CharmLocator{
		Name:     "magic",
		Revision: 2,
		Source:   domaincharm.LocalSource,
	}).Return(ioutil.NopCloser(strings.NewReader("magic content")), "hash0123", nil)
	s.charmService.EXPECT().GetCharmArchive(gomock.Any(), domaincharm.CharmLocator{
		Name:     "magic",
		Revision: 10,
		Source:   domaincharm.LocalSource,
	}).Return(ioutil.NopCloser(strings.NewReader("magic content")), "hash0123", nil)
	config := migration.UploadBinariesConfig{
		Charms: []string{
			// These 2 are out of order. Rev 2 must be uploaded first.
			"local:trusty/magic-10",
			"local:trusty/magic-2",
			"ch:trusty/postgresql-42",
		},
		CharmService:       s.charmService,
		CharmUploader:      uploader,
		Tools:              toolsMap,
		AgentBinaryStore:   s.agentBinaryStore,
		ToolsUploader:      uploader,
		Resources:          resources,
		ResourceDownloader: downloader,
		ResourceUploader:   uploader,
	}
	err := migration.UploadBinaries(c.Context(), config, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	expectedCurls := []string{
		// Note ordering.
		"ch:trusty/postgresql-42",
		"local:trusty/magic-2",
		"local:trusty/magic-10",
	}
	c.Assert(uploader.curls, tc.DeepEquals, expectedCurls)

	expectedRefs := []string{
		"postgresql-hash0123",
		"magic-hash0123",
		"magic-hash0123",
	}
	c.Assert(uploader.charmRefs, tc.DeepEquals, expectedRefs)

	c.Check(len(uploader.tools), tc.Equals, len(toolsMap))
	for _, ver := range toolsMap {
		_, exists := uploader.tools[ver]
		c.Check(exists, tc.IsTrue)
	}

	c.Assert(downloader.resources, tc.SameContents, []string{
		"app0/blob0",
		"app1/blob1",
	})
	c.Assert(uploader.resources, tc.DeepEquals, map[string]string{
		"app0/blob0": "blob0",
		"app1/blob1": "blob1",
	})
}

func (s *ImportSuite) TestWrongCharmURLAssigned(c *tc.C) {
	defer s.setupMocks(c).Finish()

	downloader := &fakeDownloader{}
	uploader := &fakeUploader{
		reassignCharmURL: true,
	}

	s.charmService.EXPECT().GetCharmArchive(gomock.Any(), domaincharm.CharmLocator{
		Name:     "bar",
		Revision: 2,
		Source:   domaincharm.CharmHubSource,
	}).Return(ioutil.NopCloser(strings.NewReader("bar content")), "hash0123", nil)
	config := migration.UploadBinariesConfig{
		Charms:             []string{"ch:foo/bar-2"},
		CharmService:       s.charmService,
		CharmUploader:      uploader,
		AgentBinaryStore:   s.agentBinaryStore,
		ToolsUploader:      uploader,
		ResourceDownloader: downloader,
		ResourceUploader:   uploader,
	}
	err := migration.UploadBinaries(c.Context(), config, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorMatches,
		"cannot upload charms: charm ch:foo/bar-2 unexpectedly assigned ch:foo/bar-100")
}

type fakeImporter struct {
	model            description.Model
	st               *state.State
	m                *state.Model
	controllerConfig controller.Config
}

func (i *fakeImporter) Import(model description.Model, controllerConfig controller.Config) (*state.Model, *state.State, error) {
	i.model = model
	i.controllerConfig = controllerConfig
	return i.m, i.st, nil
}

type fakeDownloader struct {
	uris      []string
	resources []string
}

func (d *fakeDownloader) OpenURI(_ context.Context, uri string, query url.Values) (io.ReadCloser, error) {
	if query != nil {
		panic("query should be empty")
	}
	d.uris = append(d.uris, uri)
	// Return the URI string as fake content
	return io.NopCloser(bytes.NewReader([]byte(uri))), nil
}

func (d *fakeDownloader) OpenResource(_ context.Context, app, name string) (io.ReadCloser, error) {
	d.resources = append(d.resources, app+"/"+name)
	// Use the resource name as the content.
	return io.NopCloser(bytes.NewReader([]byte(name))), nil
}

type fakeUploader struct {
	tools            map[semversion.Binary]string
	curls            []string
	charmRefs        []string
	resources        map[string]string
	reassignCharmURL bool
}

func (f *fakeUploader) UploadTools(_ context.Context, r io.Reader, v semversion.Binary) (tools.List, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	f.tools[v] = string(data)
	return tools.List{&tools.Tools{Version: v}}, nil
}

func (f *fakeUploader) UploadCharm(_ context.Context, curl string, charmRef string, r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", errors.Trace(err)
	}
	if string(data) != charm.MustParseURL(curl).Name+" content" {
		panic(fmt.Sprintf("unexpected charm body for %s: %s", curl, data))
	}
	f.curls = append(f.curls, curl)
	f.charmRefs = append(f.charmRefs, charmRef)

	outU := curl
	if f.reassignCharmURL {
		outU = charm.MustParseURL(outU).WithRevision(100).String()
	}
	return outU, nil
}

func (f *fakeUploader) UploadResource(_ context.Context, res resource.Resource, r io.Reader) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return errors.Trace(err)
	}
	f.resources[res.ApplicationName+"/"+res.Name] = string(body)
	return nil
}
