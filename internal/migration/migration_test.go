// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/core/semversion"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment/charm"
	charmresource "github.com/juju/juju/domain/deployment/charm/resource"
	"github.com/juju/juju/domain/export"
	"github.com/juju/juju/internal/docker"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/tools"
)

type ImportSuite struct {
	testhelpers.IsolationSuite
	charmService       *MockCharmService
	agentBinaryStore   *MockAgentBinaryStore
	charmUploader      *MockCharmUploader
	toolsUploader      *MockToolsUploader
	resourceDownloader *MockResourceDownloader
	resourceUploader   *MockResourceUploader
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &ImportSuite{})
}

func (s *ImportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.charmService = NewMockCharmService(ctrl)
	s.agentBinaryStore = NewMockAgentBinaryStore(ctrl)
	s.charmUploader = NewMockCharmUploader(ctrl)
	s.toolsUploader = NewMockToolsUploader(ctrl)
	s.resourceDownloader = NewMockResourceDownloader(ctrl)
	s.resourceUploader = NewMockResourceUploader(ctrl)

	return ctrl
}

func (s *ImportSuite) TestBadBytes(c *tc.C) {
	bytes := []byte("not a model")
	scope := func(model.UUID) modelmigration.Scope {
		return modelmigration.NewScope(nil, nil, nil, nil, tc.Must0(c, model.NewUUID))
	}
	importer := migration.NewModelImporter(
		scope, nil, nil,
		"controller-uuid",
		loggertesting.WrapCheckLog(c),
		clock.WallClock,
	)
	err := importer.ImportModelLegacy(c.Context(), bytes)
	c.Assert(err, tc.ErrorMatches, "yaml: unmarshal errors:\n.*")
}

func (s *ImportSuite) TestAbortModelWithoutControllerServices(c *tc.C) {
	importer := migration.NewModelImporter(
		nil, nil, nil,
		"controller-uuid",
		loggertesting.WrapCheckLog(c),
		clock.WallClock,
	)

	err := importer.AbortModel(c.Context(), model.UUID("model-uuid"))
	c.Assert(err, tc.ErrorMatches, "controller services not configured")
}

func (s *ImportSuite) TestImportModelWithoutDomainServices(c *tc.C) {
	importer := migration.NewModelImporter(
		nil, nil, nil,
		"controller-uuid",
		loggertesting.WrapCheckLog(c),
		clock.WallClock,
	)

	err := importer.ImportModel(c.Context(), migration.ImportModelArgs{}, export.ProjectionView{})
	c.Assert(err, tc.ErrorMatches, "domain services not configured")
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
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.resourceDownloader.EXPECT().OpenResource(gomock.Any(), "app0", "blob0").
		Return(io.NopCloser(strings.NewReader("blob0")), nil)
	s.resourceDownloader.EXPECT().OpenResource(gomock.Any(), "app1", "blob1").
		Return(io.NopCloser(strings.NewReader("blob1")), nil)

	toolsMap := map[string]semversion.Binary{
		"439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e": semversion.MustParseBinary("2.1.0-ubuntu-amd64"),
		"c4e12eaa8a3bf7a1a3029e2cbfccb2d88f59e8efc19f2531c423ce515afcb436": semversion.MustParseBinary("2.0.0-ubuntu-amd64"),
	}

	for sha := range toolsMap {
		// Each expectation returns its own reader: UploadTools consumes
		// the body.
		s.agentBinaryStore.EXPECT().GetAgentBinaryUsingSHA256(gomock.Any(), sha).
			Return(ioutil.NopCloser(strings.NewReader("test agent data")), 15, nil)
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

	// The uploaders echo the charm URL back and record what they received,
	// mirroring the previous fake's body checks.
	var curls, charmRefs []string
	s.charmUploader.EXPECT().UploadCharm(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, curl string, charmRef string, content io.Reader) (string, error) {
			body, rErr := io.ReadAll(content)
			c.Assert(rErr, tc.ErrorIsNil)
			c.Assert(string(body), tc.Equals, charm.MustParseURL(curl).Name+" content")
			curls = append(curls, curl)
			charmRefs = append(charmRefs, charmRef)
			return curl, nil
		}).Times(3)

	var uploadedTools []semversion.Binary
	s.toolsUploader.EXPECT().UploadTools(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, r io.Reader, v semversion.Binary) (tools.List, error) {
			body, rErr := io.ReadAll(r)
			c.Assert(rErr, tc.ErrorIsNil)
			c.Check(string(body), tc.Equals, "test agent data")
			uploadedTools = append(uploadedTools, v)
			return tools.List{&tools.Tools{Version: v}}, nil
		}).Times(len(toolsMap))

	// The placeholder resource has no blob, so exactly two uploads are
	// expected.
	uploadedResources := make(map[string]string)
	s.resourceUploader.EXPECT().UploadResource(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, res resource.Resource, r io.Reader) error {
			body, rErr := io.ReadAll(r)
			c.Assert(rErr, tc.ErrorIsNil)
			uploadedResources[res.ApplicationName+"/"+res.Name] = string(body)
			return nil
		}).Times(2)

	config := migration.UploadBinariesConfig{
		Charms: []string{
			// These 2 are out of order. Rev 2 must be uploaded first.
			"local:trusty/magic-10",
			"local:trusty/magic-2",
			"ch:trusty/postgresql-42",
		},
		CharmService:       s.charmService,
		CharmUploader:      s.charmUploader,
		Tools:              toolsMap,
		AgentBinaryStore:   s.agentBinaryStore,
		ToolsUploader:      s.toolsUploader,
		Resources:          resources,
		ResourceDownloader: s.resourceDownloader,
		ResourceUploader:   s.resourceUploader,
	}
	err := migration.UploadBinaries(c.Context(), config, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	// Note ordering.
	c.Assert(curls, tc.DeepEquals, []string{
		"ch:trusty/postgresql-42",
		"local:trusty/magic-2",
		"local:trusty/magic-10",
	})
	c.Assert(charmRefs, tc.DeepEquals, []string{
		"postgresql-hash0123",
		"magic-hash0123",
		"magic-hash0123",
	})
	c.Assert(uploadedTools, tc.SameContents, toolsVersions(toolsMap))
	c.Assert(uploadedResources, tc.DeepEquals, map[string]string{
		"app0/blob0": "blob0",
		"app1/blob1": "blob1",
	})
}

func (s *ImportSuite) TestWrongCharmURLAssigned(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.charmService.EXPECT().GetCharmArchive(gomock.Any(), domaincharm.CharmLocator{
		Name:     "bar",
		Revision: 2,
		Source:   domaincharm.CharmHubSource,
	}).Return(ioutil.NopCloser(strings.NewReader("bar content")), "hash0123", nil)

	// No expectations on the other uploaders or the resource downloader:
	// the charm failure must abort before any resource is opened.
	s.charmUploader.EXPECT().UploadCharm(gomock.Any(), "ch:foo/bar-2", "bar-hash0123", gomock.Any()).
		DoAndReturn(func(_ context.Context, curl string, _ string, content io.Reader) (string, error) {
			body, rErr := io.ReadAll(content)
			c.Assert(rErr, tc.ErrorIsNil)
			c.Assert(string(body), tc.Equals, "bar content")
			// The target controller shouldn't assign a different charm URL.
			return charm.MustParseURL(curl).WithRevision(100).String(), nil
		})

	config := migration.UploadBinariesConfig{
		Charms:             []string{"ch:foo/bar-2"},
		CharmService:       s.charmService,
		CharmUploader:      s.charmUploader,
		AgentBinaryStore:   s.agentBinaryStore,
		ToolsUploader:      s.toolsUploader,
		ResourceDownloader: s.resourceDownloader,
		ResourceUploader:   s.resourceUploader,
	}
	err := migration.UploadBinaries(c.Context(), config, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorMatches,
		"cannot upload charms: charm ch:foo/bar-2 unexpectedly assigned ch:foo/bar-100")
}

func (s *ImportSuite) TestUploadResourcesFingerprintMismatch(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// The claims describe "content" (7 bytes) but the downloader serves
	// "contenx" (7 bytes), which hashes to a different fingerprint.
	res := resourcetesting.NewResource(c, nil, "contenx", "app0", "content").Resource

	s.resourceDownloader.EXPECT().OpenResource(gomock.Any(), "app0", "contenx").
		Return(io.NopCloser(strings.NewReader("contenx")), nil)
	// No UploadResource expectation: the resource must not reach the target.

	config := migration.UploadBinariesConfig{
		CharmService:       s.charmService,
		CharmUploader:      s.charmUploader,
		AgentBinaryStore:   s.agentBinaryStore,
		ToolsUploader:      s.toolsUploader,
		Resources:          []resource.Resource{res},
		ResourceDownloader: s.resourceDownloader,
		ResourceUploader:   s.resourceUploader,
	}
	err := migration.UploadBinaries(c.Context(), config, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorMatches,
		`cannot upload resources: resource "contenx" of application "app0": blob fingerprint .* does not match expected fingerprint .*`)
}

func (s *ImportSuite) TestUploadResourcesSizeMismatch(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// The claims describe "content" (7 bytes) but the downloader serves
	// "abc" (3 bytes).
	res := resourcetesting.NewResource(c, nil, "abc", "app0", "content").Resource

	s.resourceDownloader.EXPECT().OpenResource(gomock.Any(), "app0", "abc").
		Return(io.NopCloser(strings.NewReader("abc")), nil)
	// No UploadResource expectation: the resource must not reach the target.

	config := migration.UploadBinariesConfig{
		CharmService:       s.charmService,
		CharmUploader:      s.charmUploader,
		AgentBinaryStore:   s.agentBinaryStore,
		ToolsUploader:      s.toolsUploader,
		Resources:          []resource.Resource{res},
		ResourceDownloader: s.resourceDownloader,
		ResourceUploader:   s.resourceUploader,
	}
	err := migration.UploadBinaries(c.Context(), config, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorMatches,
		`cannot upload resources: resource "abc" of application "app0": blob size 3 does not match expected size 7`)
}

// TestUploadResourcesFailFastKeepsEarlierUploads pins the fail-fast,
// non-atomic semantics of the upload loop: the first (valid) resource stays
// uploaded on the target, and the error is attributed to the second
// (corrupt) resource by name.
func (s *ImportSuite) TestUploadResourcesFailFastKeepsEarlierUploads(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	okRes := resourcetesting.NewResource(c, nil, "ok-file", "app0", "ok-content").Resource
	// The claims describe "content" (7 bytes) but the downloader serves
	// "contenx" (7 bytes), which hashes to a different fingerprint.
	badRes := resourcetesting.NewResource(c, nil, "contenx", "app0", "content").Resource

	openOK := s.resourceDownloader.EXPECT().OpenResource(gomock.Any(), "app0", "ok-file").
		Return(io.NopCloser(strings.NewReader("ok-content")), nil)
	uploadOK := s.resourceUploader.EXPECT().UploadResource(gomock.Any(), okRes, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ resource.Resource, r io.Reader) error {
			body, rErr := io.ReadAll(r)
			c.Assert(rErr, tc.ErrorIsNil)
			c.Check(string(body), tc.Equals, "ok-content")
			return nil
		})
	openBad := s.resourceDownloader.EXPECT().OpenResource(gomock.Any(), "app0", "contenx").
		Return(io.NopCloser(strings.NewReader("contenx")), nil)
	gomock.InOrder(openOK, uploadOK, openBad)
	// No UploadResource expectation for badRes: the loop aborts without
	// uploading it, and without rolling back okRes.

	config := migration.UploadBinariesConfig{
		CharmService:       s.charmService,
		CharmUploader:      s.charmUploader,
		AgentBinaryStore:   s.agentBinaryStore,
		ToolsUploader:      s.toolsUploader,
		Resources:          []resource.Resource{okRes, badRes},
		ResourceDownloader: s.resourceDownloader,
		ResourceUploader:   s.resourceUploader,
	}
	err := migration.UploadBinaries(c.Context(), config, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorMatches,
		`cannot upload resources: resource "contenx" of application "app0": blob fingerprint .* does not match expected fingerprint .*`)
}

// TestUploadResourcesContainerImageValidBlob pins the source-side happy path
// for container image resources: the claims recorded in the database equal
// the canonical JSON marshalling of DockerImageDetails — what the container
// image resource store serves on Get — so the blob the source reads always
// matches them. If this ever drifts, every container image migration would
// be rejected on the source.
func (s *ImportSuite) TestUploadResourcesContainerImageValidBlob(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	details := docker.DockerImageDetails{
		RegistryPath: "url@sha256:abc123",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "testuser",
				Password: "hunter2",
			},
		},
	}
	blob, err := json.Marshal(details)
	c.Assert(err, tc.ErrorIsNil)

	res := resourcetesting.NewDockerResource(c, nil, "image", "app0", string(blob)).Resource
	res.Type = charmresource.TypeContainerImage
	// A 4.x source records the claims derived from the canonical
	// marshalling, not a placeholder.
	res.Size = int64(len(blob))
	res.Fingerprint, err = charmresource.GenerateFingerprint(strings.NewReader(string(blob)))
	c.Assert(err, tc.ErrorIsNil)

	open := s.resourceDownloader.EXPECT().OpenResource(gomock.Any(), "app0", "image").
		Return(io.NopCloser(strings.NewReader(string(blob))), nil)
	upload := s.resourceUploader.EXPECT().UploadResource(gomock.Any(), res, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ resource.Resource, r io.Reader) error {
			body, rErr := io.ReadAll(r)
			c.Assert(rErr, tc.ErrorIsNil)
			c.Check(string(body), tc.Equals, string(blob))
			return nil
		})
	gomock.InOrder(open, upload)

	config := migration.UploadBinariesConfig{
		CharmService:       s.charmService,
		CharmUploader:      s.charmUploader,
		AgentBinaryStore:   s.agentBinaryStore,
		ToolsUploader:      s.toolsUploader,
		Resources:          []resource.Resource{res},
		ResourceDownloader: s.resourceDownloader,
		ResourceUploader:   s.resourceUploader,
	}
	err = migration.UploadBinaries(c.Context(), config, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
}

// toolsVersions returns the binary versions in toolsMap as a slice for
// order-insensitive assertions.
func toolsVersions(toolsMap map[string]semversion.Binary) []semversion.Binary {
	versions := make([]semversion.Binary, 0, len(toolsMap))
	for _, v := range toolsMap {
		versions = append(versions, v)
	}
	return versions
}
