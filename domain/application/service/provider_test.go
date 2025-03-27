// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/assumes"
	corecharm "github.com/juju/juju/core/charm"
	coreconstraints "github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

type providerServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&providerServiceSuite{})

func (s *providerServiceSuite) TestCreateApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	now := ptr(s.clock.Now())
	us := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: corestatus.MessageInstallingAgent,
				Since:   now,
			},
		},
	}}
	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
			Resources: map[string]applicationcharm.Resource{
				"foo": {Name: "foo", Type: applicationcharm.ResourceTypeFile},
				"bar": {Name: "bar", Type: applicationcharm.ResourceTypeContainerImage},
				"baz": {Name: "baz", Type: applicationcharm.ResourceTypeFile},
			},
		},
		Manifest:        s.minimalManifest(),
		ReferenceName:   "ubuntu",
		Source:          applicationcharm.CharmHubSource,
		Revision:        42,
		Architecture:    architecture.ARM64,
		ObjectStoreUUID: objectStoreUUID,
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}

	app := application.AddApplicationArg{
		Charm: ch,
		CharmDownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		Platform: platform,
		Scale:    1,
		Resources: []application.AddApplicationResourceArg{
			{
				Name:   "foo",
				Origin: charmresource.OriginUpload,
			},
			{
				Name:     "bar",
				Revision: ptr(42),
				Origin:   charmresource.OriginStore,
			},
			{
				Name: "baz",
				// It is ok to not have revision with origin store in case of
				// local charms
				Revision: nil,
				Origin:   charmresource.OriginStore,
			},
		},
		StorageParentDir: application.StorageParentDir,
		EndpointBindings: map[string]network.SpaceName{
			"":         "default",
			"provider": "beta",
		},
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)

	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)

	var receivedArgs []application.AddUnitArg
	s.state.EXPECT().CreateApplication(gomock.Any(), "ubuntu", app, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, _ application.AddApplicationArg, args []application.AddUnitArg) (coreapplication.ID, error) {
		receivedArgs = args
		return id, nil
	})

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.Stable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
		Resources: map[string]charmresource.Meta{
			"foo": {Name: "foo", Type: charmresource.TypeFile},
			"bar": {Name: "bar", Type: charmresource.TypeContainerImage},
			"baz": {Name: "baz", Type: charmresource.TypeFile},
		},
	}).MinTimes(1)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		CharmObjectStoreUUID: objectStoreUUID,
		ResolvedResources: ResolvedResources{
			{
				Name:   "foo",
				Origin: charmresource.OriginUpload,
			},
			{
				Name:     "bar",
				Revision: ptr(42),
				Origin:   charmresource.OriginStore,
			},
			{
				Name:     "baz",
				Revision: nil,
				Origin:   charmresource.OriginStore,
			},
		},
		EndpointBindings: map[string]network.SpaceName{
			"":         "default",
			"provider": "beta",
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(receivedArgs, jc.DeepEquals, us)
}

func (s *providerServiceSuite) TestCreateApplicationWithApplicationStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	now := ptr(s.clock.Now())
	status := &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "active",
		Data:    []byte(`{"active":true}`),
		Since:   now,
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)

	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)

	var receivedArgs application.AddApplicationArg
	s.state.EXPECT().CreateApplication(gomock.Any(), "ubuntu", gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ string, appArgs application.AddApplicationArg, _ []application.AddUnitArg) (coreapplication.ID, error) {
		receivedArgs = appArgs
		return id, nil
	})

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.Stable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).MinTimes(1)

	_, err := s.service.CreateApplication(context.Background(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		CharmObjectStoreUUID: objectStoreUUID,
		ApplicationStatus: &corestatus.StatusInfo{
			Status:  corestatus.Active,
			Message: "active",
			Data:    map[string]interface{}{"active": true},
			Since:   now,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(receivedArgs.Status, jc.DeepEquals, status)
}

func (s *providerServiceSuite) TestCreateApplicationPendingResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
			Resources: map[string]applicationcharm.Resource{
				"foo": {Name: "foo", Type: applicationcharm.ResourceTypeFile},
			},
		},
		Manifest:        s.minimalManifest(),
		ReferenceName:   "ubuntu",
		Source:          applicationcharm.CharmHubSource,
		Revision:        42,
		Architecture:    architecture.ARM64,
		ObjectStoreUUID: objectStoreUUID,
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}

	resourceUUID := resourcetesting.GenResourceUUID(c)
	app := application.AddApplicationArg{
		Charm: ch,
		CharmDownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		Platform:         platform,
		Scale:            1,
		PendingResources: []resource.UUID{resourceUUID},
		StorageParentDir: application.StorageParentDir,
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)

	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)

	s.state.EXPECT().CreateApplication(gomock.Any(), "ubuntu", app, gomock.Any()).Return(id, nil)

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.Stable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
		Resources: map[string]charmresource.Meta{
			"foo": {Name: "foo", Type: charmresource.TypeFile},
		},
	}).MinTimes(1)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		CharmObjectStoreUUID: objectStoreUUID,
		PendingResources:     []resource.UUID{resourceUUID},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateApplicationWithInvalidApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *providerServiceSuite) TestCreateApplicationWithInvalidCharmName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "666",
	}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *providerServiceSuite) TestCreateApplicationWithInvalidReferenceName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).AnyTimes()
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{}},
	}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "666",
		DownloadInfo: &applicationcharm.DownloadInfo{
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *providerServiceSuite) TestCreateApplicationWithNoCharmName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *providerServiceSuite) TestCreateApplicationWithNoApplicationOrCharmName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *providerServiceSuite) TestCreateApplicationWithNoMeta(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(nil).MinTimes(1)

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmMetadataNotValid)
}

func (s *providerServiceSuite) TestCreateApplicationWithNoArchitecture(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{Name: "foo"}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{}},
	}).MinTimes(1)

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.Platform{Channel: "24.04", OS: "ubuntu"},
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmOriginNotValid)
}

func (s *providerServiceSuite) TestCreateApplicationWithInvalidResourcesNotAllResourcesResolved(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{Name: "foo", Resources: map[string]charmresource.Meta{
		"not-resolved": {Name: "not-resolved"},
	}}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{
			Name: "ubuntu",
			Channel: charm.Channel{
				Risk: charm.Stable,
			},
			Architectures: []string{"amd64"},
		}},
	}).MinTimes(1)

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.Local,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	},
		AddApplicationArgs{
			ReferenceName:     "foo",
			ResolvedResources: nil,
		})
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidResourceArgs)
	c.Assert(err, gc.ErrorMatches,
		"create application: charm has resources which have not provided: invalid resource args")
}

// TestCreateApplicationWithInvalidResourceBothTypes tests that resolved resources and
// pending resources are mutually exclusive.
func (s *providerServiceSuite) TestCreateApplicationWithInvalidResourceBothTypes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{Name: "foo", Resources: map[string]charmresource.Meta{
		"not-resolved": {Name: "not-resolved"},
	}}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{
			Name: "ubuntu",
			Channel: charm.Channel{
				Risk: charm.Stable,
			},
			Architectures: []string{"amd64"},
		}},
	}).MinTimes(1)

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm,
		corecharm.Origin{
			Source:   corecharm.Local,
			Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		},
		AddApplicationArgs{
			ReferenceName:     "foo",
			ResolvedResources: ResolvedResources{ResolvedResource{Name: "testme"}},
			PendingResources:  []resource.UUID{resourcetesting.GenResourceUUID(c)},
		})
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidResourceArgs)
	// There are many places where InvalidResourceArgs are returned,
	// verify we have the expected one.
	c.Assert(err, gc.ErrorMatches,
		"create application: cannot have both pending and resolved resources: invalid resource args")
}

func (s *providerServiceSuite) TestCreateApplicationWithInvalidResourcesMoreResolvedThanCharmResources(c *gc.C) {
	resources := ResolvedResources{
		{
			Name:     "not-in-charm",
			Origin:   charmresource.OriginStore,
			Revision: ptr(42),
		},
	}
	s.testCreateApplicationWithInvalidResource(c, resources)
}

func (s *providerServiceSuite) TestCreateApplicationWithInvalidResourcesUploadWithRevision(c *gc.C) {
	resources := ResolvedResources{
		{
			Name:     "Upload-revision",
			Origin:   charmresource.OriginUpload,
			Revision: ptr(42),
		},
	}
	s.testCreateApplicationWithInvalidResource(c, resources)
}

func (s *providerServiceSuite) TestCreateApplicationWithInvalidResourcesNoName(c *gc.C) {
	resources := ResolvedResources{
		{
			Origin:   charmresource.OriginStore,
			Revision: ptr(42),
		},
	}
	s.testCreateApplicationWithInvalidResource(c, resources)
}

func (s *providerServiceSuite) TestCreateApplicationWithInvalidResourcesInvalidOrigin(c *gc.C) {
	resources := ResolvedResources{
		{
			Name:   "invalid-origin",
			Origin: 42,
		},
	}
	s.testCreateApplicationWithInvalidResource(c, resources)
}

func (s *providerServiceSuite) testCreateApplicationWithInvalidResource(c *gc.C, resources ResolvedResources) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{Name: "foo"}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{
			Name: "ubuntu",
			Channel: charm.Channel{
				Risk: charm.Stable,
			},
			Architectures: []string{"amd64"},
		}},
	}).MinTimes(1)

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.Local,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	},
		AddApplicationArgs{
			ReferenceName:     "foo",
			ResolvedResources: resources,
		})
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidResourceArgs)
}

func (s *providerServiceSuite) TestCreateApplicationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)

	rErr := errors.New("boom")
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "foo", gomock.Any(), []application.AddUnitArg{}).Return(id, rErr)

	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	})
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `creating application "foo": boom`)
}

func (s *providerServiceSuite) TestCreateApplicationWithStorageBlock(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	now := ptr(s.clock.Now())
	us := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: "waiting for machine",
				Since:   now,
			},
		},
	}}
	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]applicationcharm.Storage{
				"data": {
					Name:        "data",
					Type:        applicationcharm.StorageBlock,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
		Manifest:      s.minimalManifest(),
		ReferenceName: "foo",
		Source:        applicationcharm.LocalSource,
		Revision:      42,
		Architecture:  architecture.AMD64,
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       application.Ubuntu,
		Architecture: architecture.AMD64,
	}
	app := application.AddApplicationArg{
		Charm: ch,
		CharmDownloadInfo: &applicationcharm.DownloadInfo{
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		Platform: platform,
		Storage: []application.ApplicationStorageArg{{
			Name:           "data",
			PoolNameOrType: "loop",
			Size:           10,
			Count:          1,
		}},
		Scale: 1,
		StoragePoolKind: map[string]storage.StorageKind{
			"loop": storage.StorageKindBlock,
		},
		StorageParentDir: application.StorageParentDir,
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)

	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "foo", app, us).Return(id, nil)

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
		Storage: map[string]charm.Storage{
			"data": {
				Name:        "data",
				Type:        charm.StorageBlock,
				Shared:      false,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 10,
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	pool := domainstorage.StoragePoolDetails{Name: "loop", Provider: "loop"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "loop").Return(pool, nil).MaxTimes(2)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.Local,
		Platform: corecharm.MustParsePlatform("amd64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateApplicationWithStorageBlockDefaultSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	now := ptr(s.clock.Now())
	us := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: corestatus.MessageWaitForMachine,
				Since:   now,
			},
		},
	}}
	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]applicationcharm.Storage{
				"data": {
					Name:        "data",
					Type:        applicationcharm.StorageBlock,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
		Manifest:      s.minimalManifest(),
		ReferenceName: "foo",
		Source:        applicationcharm.CharmHubSource,
		Revision:      42,
		Architecture:  architecture.AMD64,
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       application.Ubuntu,
		Architecture: architecture.AMD64,
	}
	app := application.AddApplicationArg{
		Charm: ch,
		CharmDownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		Platform: platform,
		Storage: []application.ApplicationStorageArg{{
			Name:           "data",
			PoolNameOrType: "fast",
			Size:           10,
			Count:          2,
		}},
		Scale: 1,
		StoragePoolKind: map[string]storage.StorageKind{
			"fast": storage.StorageKindBlock,
		},
		StorageParentDir: application.StorageParentDir,
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)

	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{DefaultBlockSource: ptr("fast")}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "foo", app, us).Return(id, nil)

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
		Storage: map[string]charm.Storage{
			"data": {
				Name:        "data",
				Type:        charm.StorageBlock,
				Shared:      false,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 10,
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	pool := domainstorage.StoragePoolDetails{Name: "fast", Provider: "modelscoped-block"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "fast").Return(pool, nil).MaxTimes(2)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("amd64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		Storage: map[string]storage.Directive{
			"data": {Count: 2},
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateApplicationWithStorageFilesystem(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	now := ptr(s.clock.Now())
	us := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: "waiting for machine",
				Since:   now,
			},
		},
	}}
	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]applicationcharm.Storage{
				"data": {
					Name:        "data",
					Type:        applicationcharm.StorageFilesystem,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
		Manifest:      s.minimalManifest(),
		ReferenceName: "foo",
		Source:        applicationcharm.CharmHubSource,
		Revision:      42,
		Architecture:  architecture.AMD64,
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       application.Ubuntu,
		Architecture: architecture.AMD64,
	}
	app := application.AddApplicationArg{
		Charm: ch,
		CharmDownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		Platform: platform,
		Storage: []application.ApplicationStorageArg{{
			Name:           "data",
			PoolNameOrType: "rootfs",
			Size:           10,
			Count:          1,
		}},
		Scale: 1,
		StoragePoolKind: map[string]storage.StorageKind{
			"rootfs": storage.StorageKindFilesystem,
		},
		StorageParentDir: application.StorageParentDir,
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)

	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "foo", app, us).Return(id, nil)

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
		Storage: map[string]charm.Storage{
			"data": {
				Name:        "data",
				Type:        charm.StorageFilesystem,
				Shared:      false,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 10,
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	pool := domainstorage.StoragePoolDetails{Name: "rootfs", Provider: "rootfs"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "rootfs").Return(pool, nil).MaxTimes(2)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("amd64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateApplicationWithStorageFilesystemDefaultSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	now := ptr(s.clock.Now())
	us := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: "waiting for machine",
				Since:   now,
			},
		},
	}}
	ch := applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]applicationcharm.Storage{
				"data": {
					Name:        "data",
					Type:        applicationcharm.StorageFilesystem,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
		Manifest:      s.minimalManifest(),
		ReferenceName: "foo",
		Source:        applicationcharm.CharmHubSource,
		Revision:      42,
		Architecture:  architecture.AMD64,
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       application.Ubuntu,
		Architecture: architecture.AMD64,
	}
	app := application.AddApplicationArg{
		Charm: ch,
		CharmDownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		Platform: platform,
		Storage: []application.ApplicationStorageArg{{
			Name:           "data",
			PoolNameOrType: "fast",
			Size:           10,
			Count:          2,
		}},
		Scale: 1,
		StoragePoolKind: map[string]storage.StorageKind{
			"fast": storage.StorageKindBlock,
		},
		StorageParentDir: application.StorageParentDir,
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)

	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{DefaultFilesystemSource: ptr("fast")}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "foo", app, us).Return(id, nil)

	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
		Storage: map[string]charm.Storage{
			"data": {
				Name:        "data",
				Type:        charm.StorageFilesystem,
				Shared:      false,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 10,
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	pool := domainstorage.StoragePoolDetails{Name: "fast", Provider: "modelscoped"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "fast").Return(pool, nil).MaxTimes(2)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("amd64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		Storage: map[string]storage.Directive{
			"data": {Count: 2},
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateApplicationWithSharedStorageMissingDirectives(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
		Storage: map[string]charm.Storage{
			"data": {
				Name:   "data",
				Type:   charm.StorageBlock,
				Shared: true,
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}, a)
	c.Assert(err, jc.ErrorIs, storageerrors.MissingSharedStorageDirectiveError)
	c.Assert(err, gc.ErrorMatches, `.*adding default storage directives: no storage directive specified for shared charm storage "data"`)
}

func (s *providerServiceSuite) TestCreateApplicationWithStorageValidates(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "mine",
		Storage: map[string]charm.Storage{
			"data": {
				Name: "data",
				Type: charm.StorageBlock,
			},
		},
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{{
		Name:          "ubuntu",
		Channel:       charm.Channel{Risk: charm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	// Depending on the map serialization order, the loop may or may not be the
	// first element. In that case, we need to handle it with a mock if it is
	// called. We only ever expect it to be called a maximum of once.
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "loop").
		Return(domainstorage.StoragePoolDetails{}, storageerrors.PoolNotFoundError).MaxTimes(1)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		DownloadInfo: &applicationcharm.DownloadInfo{
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
		Storage: map[string]storage.Directive{
			"logs": {Count: 2},
		},
	}, a)
	c.Assert(err, gc.ErrorMatches, `.*invalid storage directives: charm "mine" has no store called "logs"`)
}

func (s *providerServiceSuite) TestGetSupportedFeatures(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentVersion := semversion.MustParse("4.0.0")
	s.agentVersionGetter.EXPECT().GetTargetAgentVersion(gomock.Any()).Return(agentVersion, nil)

	s.supportedFeaturesProvider.EXPECT().SupportedFeatures().Return(assumes.FeatureSet{}, nil)

	features, err := s.service.GetSupportedFeatures(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{
		Name:        "juju",
		Description: assumes.UserFriendlyFeatureDescriptions["juju"],
		Version:     &agentVersion,
	})
	c.Check(features, jc.DeepEquals, fs)
}

func (s *providerServiceSuite) TestGetSupportedFeaturesNotSupported(c *gc.C) {
	ctrl := s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, coreerrors.NotSupported
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, coreerrors.NotSupported
	})
	defer ctrl.Finish()

	agentVersion := semversion.MustParse("4.0.0")
	s.agentVersionGetter.EXPECT().GetTargetAgentVersion(gomock.Any()).Return(agentVersion, nil)

	features, err := s.service.GetSupportedFeatures(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{
		Name:        "juju",
		Description: assumes.UserFriendlyFeatureDescriptions["juju"],
		Version:     &agentVersion,
	})
	c.Check(features, jc.DeepEquals, fs)
}

func (s *providerServiceSuite) TestGetApplicationConstraintsInvalidAppID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationConstraints(context.Background(), "bad-app-id")
	c.Assert(err, gc.ErrorMatches, "application ID: id \"bad-app-id\" not valid")
}

func (s *providerServiceSuite) TestSetApplicationConstraintsInvalidAppID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetApplicationConstraints(context.Background(), "bad-app-id", coreconstraints.Value{})
	c.Assert(err, gc.ErrorMatches, "application ID: id \"bad-app-id\" not valid")
}

func (s *providerServiceSuite) TestSetConstraintsProviderNotSupported(c *gc.C) {
	ctrl := s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, coreerrors.NotSupported
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, coreerrors.NotSupported
	})
	defer ctrl.Finish()

	id := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().SetApplicationConstraints(gomock.Any(), id, constraints.Constraints{}).Return(nil)

	err := s.service.SetApplicationConstraints(context.Background(), id, coreconstraints.Value{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerServiceSuite) TestSetConstraintsValidatorError(c *gc.C) {
	ctrl := s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	})
	defer ctrl.Finish()

	id := applicationtesting.GenApplicationUUID(c)

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, errors.New("boom"))

	err := s.service.SetApplicationConstraints(context.Background(), id, coreconstraints.Value{})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *providerServiceSuite) TestSetConstraintsValidateError(c *gc.C) {
	ctrl := s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	})
	defer ctrl.Finish()

	id := applicationtesting.GenApplicationUUID(c)

	validator := NewMockValidator(ctrl)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(validator, nil)
	validator.EXPECT().Validate(gomock.Any()).Return(nil, errors.New("boom"))

	err := s.service.SetApplicationConstraints(context.Background(), id, coreconstraints.Value{})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *providerServiceSuite) TestSetConstraintsUnsupportedValues(c *gc.C) {
	ctrl := s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	})
	defer ctrl.Finish()

	id := applicationtesting.GenApplicationUUID(c)

	validator := NewMockValidator(ctrl)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(validator, nil)
	validator.EXPECT().Validate(gomock.Any()).Return([]string{"arch", "mem"}, nil)
	s.state.EXPECT().SetApplicationConstraints(gomock.Any(), id, constraints.Constraints{Arch: ptr("amd64"), Mem: ptr(uint64(8))}).Return(nil)

	err := s.service.SetApplicationConstraints(context.Background(), id, coreconstraints.Value{Arch: ptr("amd64"), Mem: ptr(uint64(8))})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(c.GetTestLog(), jc.Contains, "unsupported constraints: arch,mem")
}

func (s *providerServiceSuite) TestSetConstraints(c *gc.C) {
	ctrl := s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	})
	defer ctrl.Finish()

	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	validator := NewMockValidator(ctrl)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(validator, nil)
	validator.EXPECT().Validate(gomock.Any()).Return(nil, nil)
	s.state.EXPECT().SetApplicationConstraints(gomock.Any(), id, constraints.Constraints{Arch: ptr("amd64"), Mem: ptr(uint64(8))}).Return(nil)

	err := s.service.SetApplicationConstraints(context.Background(), id, coreconstraints.Value{Arch: ptr("amd64"), Mem: ptr(uint64(8))})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerServiceSuite) TestAddUnitsEmptyConstraints(c *gc.C) {
	defer s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	}).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)

	now := ptr(s.clock.Now())
	u := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: corestatus.MessageInstallingAgent,
				Since:   now,
			},
		},
	}}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	s.expectEmptyUnitConstraints(c, unitUUID, appUUID)

	var received []application.AddUnitArg
	s.state.EXPECT().AddCAASUnits(gomock.Any(), s.storageParentDir, appUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, _ coreapplication.ID, args ...application.AddUnitArg) error {
		received = args
		return nil
	})

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), s.storageParentDir, "ubuntu", a)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, jc.DeepEquals, u)
}

func (s *providerServiceSuite) TestAddUnitsAppConstraints(c *gc.C) {
	defer s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	}).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)

	now := ptr(s.clock.Now())
	u := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		Constraints: constraints.Constraints{
			Arch:           ptr("amd64"),
			Container:      ptr(instance.LXD),
			CpuCores:       ptr(uint64(4)),
			Mem:            ptr(uint64(1024)),
			RootDisk:       ptr(uint64(1024)),
			RootDiskSource: ptr("root-disk-source"),
			Tags:           ptr([]string{"tag1", "tag2"}),
			InstanceRole:   ptr("instance-role"),
			InstanceType:   ptr("instance-type"),
			Spaces: ptr([]constraints.SpaceConstraint{
				{SpaceName: "space1", Exclude: false},
			}),
			VirtType:         ptr("virt-type"),
			Zones:            ptr([]string{"zone1", "zone2"}),
			AllocatePublicIP: ptr(true),
		},
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: corestatus.MessageInstallingAgent,
				Since:   now,
			},
		},
	}}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	s.expectAppConstraints(c, unitUUID, appUUID)

	var received []application.AddUnitArg
	s.state.EXPECT().AddCAASUnits(gomock.Any(), s.storageParentDir, appUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, _ coreapplication.ID, args ...application.AddUnitArg) error {
		received = args
		return nil
	})

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), s.storageParentDir, "ubuntu", a)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, jc.DeepEquals, u)
}

func (s *providerServiceSuite) TestAddUnitsModelConstraints(c *gc.C) {
	defer s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	}).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)

	now := ptr(s.clock.Now())
	u := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		Constraints: constraints.Constraints{
			Arch:           ptr("amd64"),
			Container:      ptr(instance.LXD),
			CpuCores:       ptr(uint64(4)),
			Mem:            ptr(uint64(1024)),
			RootDisk:       ptr(uint64(1024)),
			RootDiskSource: ptr("root-disk-source"),
			Tags:           ptr([]string{"tag1", "tag2"}),
			InstanceRole:   ptr("instance-role"),
			InstanceType:   ptr("instance-type"),
			Spaces: ptr([]constraints.SpaceConstraint{
				{SpaceName: "space1", Exclude: false},
			}),
			VirtType:         ptr("virt-type"),
			Zones:            ptr([]string{"zone1", "zone2"}),
			AllocatePublicIP: ptr(true),
		},
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: corestatus.MessageInstallingAgent,
				Since:   now,
			},
		},
	}}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	s.expectModelConstraints(c, unitUUID, appUUID)

	var received []application.AddUnitArg
	s.state.EXPECT().AddCAASUnits(gomock.Any(), s.storageParentDir, appUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, _ coreapplication.ID, args ...application.AddUnitArg) error {
		received = args
		return nil
	})

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), s.storageParentDir, "ubuntu", a)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, jc.DeepEquals, u)
}

func (s *providerServiceSuite) TestAddUnitsFullConstraints(c *gc.C) {
	defer s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	}).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)

	now := ptr(s.clock.Now())
	u := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		Constraints: constraints.Constraints{
			CpuCores: ptr(uint64(4)),
			CpuPower: ptr(uint64(75)),
		},
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: corestatus.MessageInstallingAgent,
				Since:   now,
			},
		},
	}}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	s.expectFullConstraints(c, unitUUID, appUUID)

	var received []application.AddUnitArg
	s.state.EXPECT().AddCAASUnits(gomock.Any(), s.storageParentDir, appUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, _ coreapplication.ID, args ...application.AddUnitArg) error {
		received = args
		return nil
	})

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), s.storageParentDir, "ubuntu", a)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, jc.DeepEquals, u)
}

func (s *providerServiceSuite) TestAddUnitsInvalidName(c *gc.C) {
	defer s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	}).Finish()

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), s.storageParentDir, "!!!", a)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *providerServiceSuite) TestAddUnitsNoUnits(c *gc.C) {
	defer s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	}).Finish()

	err := s.service.AddUnits(context.Background(), s.storageParentDir, "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerServiceSuite) TestAddUnitsApplicationNotFound(c *gc.C) {
	defer s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	}).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, applicationerrors.ApplicationNotFound)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), s.storageParentDir, "ubuntu", a)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *providerServiceSuite) TestAddUnitsGetModelTypeError(c *gc.C) {
	defer s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	}).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", errors.Errorf("boom"))
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), s.storageParentDir, "ubuntu", a)
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, errors.Errorf("not supported %w", coreerrors.NotSupported))

	_, err := s.service.mergeApplicationAndModelConstraints(context.Background(), constraints.Constraints{})
	c.Assert(err, jc.ErrorIs, coreerrors.NotSupported)
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsNilValidator(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)

	cons, err := s.service.mergeApplicationAndModelConstraints(context.Background(), constraints.Constraints{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, gc.DeepEquals, coreconstraints.Value{})
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsConstraintsNotFound(c *gc.C) {
	defer s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	}).Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, modelerrors.ConstraintsNotFound)

	s.validator.EXPECT().Merge(
		constraints.EncodeConstraints(constraints.Constraints{}),
		constraints.EncodeConstraints(constraints.Constraints{})).
		Return(coreconstraints.Value{}, nil)

	_, err := s.service.mergeApplicationAndModelConstraints(context.Background(), constraints.Constraints{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerServiceSuite) expectEmptyUnitConstraints(c *gc.C, unitUUID coreunit.UUID, appUUID coreapplication.ID) {
	appConstraints := constraints.Constraints{}
	modelConstraints := constraints.Constraints{}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(appConstraints, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)

	s.validator.EXPECT().Merge(constraints.EncodeConstraints(appConstraints), constraints.EncodeConstraints(modelConstraints)).Return(coreconstraints.Value{}, nil)
}

func (s *providerServiceSuite) expectAppConstraints(c *gc.C, unitUUID coreunit.UUID, appUUID coreapplication.ID) {
	appConstraints := constraints.Constraints{
		Arch:           ptr("amd64"),
		Container:      ptr(instance.LXD),
		CpuCores:       ptr(uint64(4)),
		Mem:            ptr(uint64(1024)),
		RootDisk:       ptr(uint64(1024)),
		RootDiskSource: ptr("root-disk-source"),
		Tags:           ptr([]string{"tag1", "tag2"}),
		InstanceRole:   ptr("instance-role"),
		InstanceType:   ptr("instance-type"),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
		VirtType:         ptr("virt-type"),
		Zones:            ptr([]string{"zone1", "zone2"}),
		AllocatePublicIP: ptr(true),
	}
	modelConstraints := constraints.Constraints{}
	unitConstraints := appConstraints

	s.validator.EXPECT().Merge(constraints.EncodeConstraints(appConstraints), constraints.EncodeConstraints(modelConstraints)).
		Return(constraints.EncodeConstraints(unitConstraints), nil)

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(appConstraints, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)
}

func (s *providerServiceSuite) expectModelConstraints(c *gc.C, unitUUID coreunit.UUID, appUUID coreapplication.ID) {
	modelConstraints := constraints.Constraints{
		Arch:           ptr("amd64"),
		Container:      ptr(instance.LXD),
		CpuCores:       ptr(uint64(4)),
		Mem:            ptr(uint64(1024)),
		RootDisk:       ptr(uint64(1024)),
		RootDiskSource: ptr("root-disk-source"),
		Tags:           ptr([]string{"tag1", "tag2"}),
		InstanceRole:   ptr("instance-role"),
		InstanceType:   ptr("instance-type"),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
		VirtType:         ptr("virt-type"),
		Zones:            ptr([]string{"zone1", "zone2"}),
		AllocatePublicIP: ptr(true),
	}
	appConstraints := constraints.Constraints{}
	unitConstraints := modelConstraints

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(appConstraints, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)

	s.validator.EXPECT().Merge(constraints.EncodeConstraints(appConstraints), constraints.EncodeConstraints(modelConstraints)).Return(constraints.EncodeConstraints(unitConstraints), nil)
}

func (s *providerServiceSuite) expectFullConstraints(c *gc.C, unitUUID coreunit.UUID, appUUID coreapplication.ID) {
	modelConstraints := constraints.Constraints{
		CpuCores: ptr(uint64(4)),
	}
	appConstraints := constraints.Constraints{
		CpuPower: ptr(uint64(75)),
	}
	unitConstraints := constraints.Constraints{
		CpuCores: ptr(uint64(4)),
		CpuPower: ptr(uint64(75)),
	}

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.validator.EXPECT().Merge(constraints.EncodeConstraints(appConstraints), constraints.EncodeConstraints(modelConstraints)).Return(constraints.EncodeConstraints(unitConstraints), nil)

	s.state.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(appConstraints, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)
}
