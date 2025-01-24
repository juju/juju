// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/juju/clock/testclock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/config"
	modeltesting "github.com/juju/juju/core/model/testing"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/charm/store"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	"github.com/juju/juju/testcharms"
)

type applicationServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&applicationServiceSuite{})

func (s *applicationServiceSuite) TestCreateApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	u := application.AddUnitArg{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusAllocating,
				StatusInfo: application.StatusInfo{
					Since: s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusWaiting,
				StatusInfo: application.StatusInfo{
					Message: "installing agent",
					Since:   s.clock.Now(),
				},
			},
		},
	}
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
		Manifest:        s.minimalManifest(c),
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
	}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "ubuntu", app).Return(id, nil)
	s.state.EXPECT().AddUnits(domaintesting.IsAtomicContextChecker, id, u)

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
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateApplicationWithInvalidApplicationName(c *gc.C) {
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

func (s *applicationServiceSuite) TestCreateApplicationWithInvalidCharmName(c *gc.C) {
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

func (s *applicationServiceSuite) TestCreateApplicationWithInvalidReferenceName(c *gc.C) {
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

func (s *applicationServiceSuite) TestCreateApplicationWithNoCharmName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationWithNoApplicationOrCharmName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationWithNoMeta(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(nil).MinTimes(1)

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmMetadataNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationWithNoArchitecture(c *gc.C) {
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

func (s *applicationServiceSuite) TestCreateApplicationWithInvalidResourcesNotAllResourcesResolved(c *gc.C) {
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
}

func (s *applicationServiceSuite) TestCreateApplicationWithInvalidResourcesMoreResolvedThanCharmResources(c *gc.C) {
	resources := ResolvedResources{
		{
			Name:     "not-in-charm",
			Origin:   charmresource.OriginStore,
			Revision: ptr(42),
		},
	}
	s.testCreateApplicationWithInvalidResource(c, resources)
}

func (s *applicationServiceSuite) TestCreateApplicationWithInvalidResourcesUploadWithRevision(c *gc.C) {
	resources := ResolvedResources{
		{
			Name:     "Upload-revision",
			Origin:   charmresource.OriginUpload,
			Revision: ptr(42),
		},
	}
	s.testCreateApplicationWithInvalidResource(c, resources)
}

func (s *applicationServiceSuite) TestCreateApplicationWithInvalidResourcesNoName(c *gc.C) {
	resources := ResolvedResources{
		{
			Origin:   charmresource.OriginStore,
			Revision: ptr(42),
		},
	}
	s.testCreateApplicationWithInvalidResource(c, resources)
}

func (s *applicationServiceSuite) TestCreateApplicationWithInvalidResourcesInvalidOrigin(c *gc.C) {
	resources := ResolvedResources{
		{
			Name:   "invalid-origin",
			Origin: 42,
		},
	}
	s.testCreateApplicationWithInvalidResource(c, resources)
}

func (s *applicationServiceSuite) testCreateApplicationWithInvalidResource(c *gc.C, resources ResolvedResources) {
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

func (s *applicationServiceSuite) TestCreateApplicationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	rErr := errors.New("boom")
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "foo", gomock.Any()).Return(id, rErr)

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

func (s *applicationServiceSuite) TestCreateWithStorageBlock(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.AddUnitArg{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusAllocating,
				StatusInfo: application.StatusInfo{
					Since: s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusWaiting,
				StatusInfo: application.StatusInfo{
					Message: "waiting for machine",
					Since:   s.clock.Now(),
				},
			},
		},
	}
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
		Manifest:      s.minimalManifest(c),
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
		Scale:    1,
	}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "foo", app).Return(id, nil)
	s.state.EXPECT().AddUnits(domaintesting.IsAtomicContextChecker, id, u)

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
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "loop").Return(pool, nil)

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

func (s *applicationServiceSuite) TestCreateWithStorageBlockDefaultSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.AddUnitArg{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusAllocating,
				StatusInfo: application.StatusInfo{
					Since: s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusWaiting,
				StatusInfo: application.StatusInfo{
					Message: "waiting for machine",
					Since:   s.clock.Now(),
				},
			},
		},
	}
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
		Manifest:      s.minimalManifest(c),
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
		Scale:    1,
	}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{DefaultBlockSource: ptr("fast")}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "foo", app).Return(id, nil)
	s.state.EXPECT().AddUnits(domaintesting.IsAtomicContextChecker, id, u)

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
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "fast").Return(pool, nil)

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

func (s *applicationServiceSuite) TestCreateWithStorageFilesystem(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.AddUnitArg{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusAllocating,
				StatusInfo: application.StatusInfo{
					Since: s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusWaiting,
				StatusInfo: application.StatusInfo{
					Message: "waiting for machine",
					Since:   s.clock.Now(),
				},
			},
		},
	}
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
		Manifest:      s.minimalManifest(c),
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
		Scale:    1,
	}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "foo", app).Return(id, nil)
	s.state.EXPECT().AddUnits(domaintesting.IsAtomicContextChecker, id, u)

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
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "rootfs").Return(pool, nil)

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

func (s *applicationServiceSuite) TestCreateWithStorageFilesystemDefaultSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.AddUnitArg{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusAllocating,
				StatusInfo: application.StatusInfo{
					Since: s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusWaiting,
				StatusInfo: application.StatusInfo{
					Message: "waiting for machine",
					Since:   s.clock.Now(),
				},
			},
		},
	}
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
		Manifest:      s.minimalManifest(c),
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
		Scale:    1,
	}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{DefaultFilesystemSource: ptr("fast")}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "foo", app).Return(id, nil)
	s.state.EXPECT().AddUnits(domaintesting.IsAtomicContextChecker, id, u)

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
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "fast").Return(pool, nil)

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

func (s *applicationServiceSuite) TestCreateWithSharedStorageMissingDirectives(c *gc.C) {
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

func (s *applicationServiceSuite) TestCreateWithStorageValidates(c *gc.C) {
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

func (s *applicationServiceSuite) TestAddUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.AddUnitArg{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusAllocating,
				StatusInfo: application.StatusInfo{
					Since: s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusWaiting,
				StatusInfo: application.StatusInfo{
					Message: "installing agent",
					Since:   s.clock.Now(),
				},
			},
		},
	}
	appID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().GetApplicationID(domaintesting.IsAtomicContextChecker, "666").Return(appID, nil)
	s.state.EXPECT().AddUnits(domaintesting.IsAtomicContextChecker, appID, u).Return(nil)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), "666", a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestGetUnitUUIDs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	names := []coreunit.Name{coreunit.Name("foo/666"), coreunit.Name("foo/667")}
	uuids := []coreunit.UUID{unittesting.GenUnitUUID(c), unittesting.GenUnitUUID(c)}
	s.state.EXPECT().GetUnitUUIDs(gomock.Any(), names).Return(uuids, nil)

	us, err := s.service.GetUnitUUIDs(context.Background(), names)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(us, gc.DeepEquals, uuids)
}

func (s *applicationServiceSuite) TestGetUnitUUIDsErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	names := []coreunit.Name{coreunit.Name("foo/666"), coreunit.Name("foo/667")}
	s.state.EXPECT().GetUnitUUIDs(gomock.Any(), names).Return(nil, applicationerrors.UnitNotFound)

	_, err := s.service.GetUnitUUIDs(context.Background(), names)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationServiceSuite) TestGetUnitUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := unittesting.GenUnitUUID(c)
	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetUnitUUIDs(gomock.Any(), []coreunit.Name{unitName}).Return([]coreunit.UUID{uuid}, nil)

	u, err := s.service.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u, gc.Equals, uuid)
}

func (s *applicationServiceSuite) TestGetUnitUUIDErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetUnitUUIDs(gomock.Any(), []coreunit.Name{unitName}).Return(nil, applicationerrors.UnitNotFound)

	_, err := s.service.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationServiceSuite) TestGetUnitNames(c *gc.C) {
	defer s.setupMocks(c).Finish()

	names := []coreunit.Name{coreunit.Name("foo/666"), coreunit.Name("foo/667")}
	uuids := []coreunit.UUID{unittesting.GenUnitUUID(c), unittesting.GenUnitUUID(c)}
	s.state.EXPECT().GetUnitNames(gomock.Any(), uuids).Return(names, nil)

	u, err := s.service.GetUnitNames(context.Background(), uuids)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u, gc.DeepEquals, names)
}

func (s *applicationServiceSuite) TestGetUnitNamesErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuids := []coreunit.UUID{unittesting.GenUnitUUID(c), unittesting.GenUnitUUID(c)}
	s.state.EXPECT().GetUnitNames(gomock.Any(), uuids).Return(nil, applicationerrors.UnitNotFound)

	_, err := s.service.GetUnitNames(context.Background(), uuids)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationServiceSuite) TestRegisterCAASUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")

	s.state.EXPECT().GetApplicationID(domaintesting.IsAtomicContextChecker, "foo").Return("app-id", nil)
	s.state.EXPECT().GetUnitLife(domaintesting.IsAtomicContextChecker, unitName).Return(life.Alive, nil)
	s.state.EXPECT().UpdateUnitContainer(domaintesting.IsAtomicContextChecker, unitName, &application.CloudContainer{
		ProviderId: "provider-id",
		Address: &application.ContainerAddress{
			Device: application.ContainerDevice{
				Name:              `placeholder for "foo/666" cloud container`,
				DeviceTypeID:      0,
				VirtualPortTypeID: 0,
			},
			Value:       "10.6.6.6",
			AddressType: 0,
			Scope:       3,
			Origin:      1,
			ConfigType:  1,
		},
		Ports: ptr([]string{"666"}),
	})
	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUID(domaintesting.IsAtomicContextChecker, unitName).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitPassword(domaintesting.IsAtomicContextChecker, unitUUID, application.PasswordInfo{
		PasswordHash:  "passwordhash",
		HashAlgorithm: 0,
	})

	p := RegisterCAASUnitParams{
		UnitName:     unitName,
		PasswordHash: "passwordhash",
		ProviderId:   "provider-id",
		Address:      ptr("10.6.6.6"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    1,
	}
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIsNil)
}

var unitParams = RegisterCAASUnitParams{
	UnitName:     coreunit.Name("foo/666"),
	PasswordHash: "passwordhash",
	ProviderId:   "provider-id",
	OrderedScale: true,
	OrderedId:    1,
}

func (s *applicationServiceSuite) TestRegisterCAASUnitMissingUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.UnitName = ""
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "missing unit name not valid")
}

func (s *applicationServiceSuite) TestRegisterCAASUnitMissingOrderedScale(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.OrderedScale = false
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "registering CAAS units not supported without ordered unit IDs")
}

func (s *applicationServiceSuite) TestRegisterCAASUnitMissingProviderID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.ProviderId = ""
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "provider id not valid")
}

func (s *applicationServiceSuite) TestRegisterCAASUnitMissingPasswordHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.PasswordHash = ""
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "password hash not valid")
}

func (s *applicationServiceSuite) TestUpdateCAASUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)
	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetApplicationLife(domaintesting.IsAtomicContextChecker, "foo").Return(appID, life.Alive, nil)
	s.state.EXPECT().UpdateUnitContainer(domaintesting.IsAtomicContextChecker, unitName, &application.CloudContainer{
		ProviderId: "provider-id",
		Address: &application.ContainerAddress{
			Device: application.ContainerDevice{
				Name:              `placeholder for "foo/666" cloud container`,
				DeviceTypeID:      0,
				VirtualPortTypeID: 0,
			},
			Value:       "10.6.6.6",
			AddressType: 0,
			Scope:       3,
			Origin:      1,
			ConfigType:  1,
		},
		Ports: ptr([]string{"666"}),
	})
	s.state.EXPECT().GetUnitUUID(domaintesting.IsAtomicContextChecker, unitName).Return(unitUUID, nil)

	now := time.Now()
	s.state.EXPECT().SetUnitAgentStatus(domaintesting.IsAtomicContextChecker, unitUUID, application.UnitAgentStatusInfo{
		StatusID: application.UnitAgentStatusIdle,
		StatusInfo: application.StatusInfo{
			Message: "agent status",
			Data:    map[string]string{"foo": "bar"},
			Since:   now,
		},
	})
	s.state.EXPECT().SetUnitWorkloadStatus(domaintesting.IsAtomicContextChecker, unitUUID, application.UnitWorkloadStatusInfo{
		StatusID: application.UnitWorkloadStatusWaiting,
		StatusInfo: application.StatusInfo{
			Message: "workload status",
			Data:    map[string]string{"foo": "bar"},
			Since:   now,
		},
	})
	s.state.EXPECT().SetCloudContainerStatus(domaintesting.IsAtomicContextChecker, unitUUID, application.CloudContainerStatusStatusInfo{
		StatusID: application.CloudContainerStatusRunning,
		StatusInfo: application.StatusInfo{
			Message: "container status",
			Data:    map[string]string{"foo": "bar"},
			Since:   now,
		},
	})

	err := s.service.UpdateCAASUnit(context.Background(), unitName, UpdateCAASUnitParams{
		ProviderId: ptr("provider-id"),
		Address:    ptr("10.6.6.6"),
		Ports:      ptr([]string{"666"}),
		AgentStatus: ptr(StatusParams{
			Status:  "idle",
			Message: "agent status",
			Data:    map[string]any{"foo": "bar"},
			Since:   ptr(now),
		}),
		WorkloadStatus: ptr(StatusParams{
			Status:  "waiting",
			Message: "workload status",
			Data:    map[string]any{"foo": "bar"},
			Since:   ptr(now),
		}),
		CloudContainerStatus: ptr(StatusParams{
			Status:  "running",
			Message: "container status",
			Data:    map[string]any{"foo": "bar"},
			Since:   ptr(now),
		}),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUpdateCAASUnitNotAlive(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationLife(domaintesting.IsAtomicContextChecker, "foo").Return(id, life.Dying, nil)

	err := s.service.UpdateCAASUnit(context.Background(), coreunit.Name("foo/666"), UpdateCAASUnitParams{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *applicationServiceSuite) TestSetUnitPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUID(domaintesting.IsAtomicContextChecker, coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitPassword(domaintesting.IsAtomicContextChecker, unitUUID, application.PasswordInfo{
		PasswordHash:  "password",
		HashAlgorithm: 0,
	})

	err := s.service.SetUnitPassword(context.Background(), coreunit.Name("foo/666"), "password")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestGetCharmByApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetCharmByApplicationID(gomock.Any(), id).Return(applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name: "foo",

			// RunAs becomes mandatory when being persisted. Empty string is not
			// allowed.
			RunAs: "default",
		},
		ReferenceName: "bar",
		Revision:      42,
		Source:        applicationcharm.LocalSource,
		Architecture:  architecture.AMD64,
	}, nil)

	ch, locator, err := s.service.GetCharmByApplicationID(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch.Meta(), gc.DeepEquals, &charm.Meta{
		Name: "foo",

		// Notice that the RunAs field becomes empty string when being returned.
	})
	c.Check(locator, gc.DeepEquals, applicationcharm.CharmLocator{
		Name:         "bar",
		Revision:     42,
		Source:       applicationcharm.LocalSource,
		Architecture: architecture.AMD64,
	})
}

func (s *applicationServiceSuite) TestGetCharmIDByApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)

	charmID, err := s.service.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(charmID, gc.DeepEquals, id)
}

func (s *applicationServiceSuite) TestGetApplicationIDByUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expectedAppID := applicationtesting.GenApplicationUUID(c)
	unitName := coreunit.Name("foo")
	s.state.EXPECT().GetApplicationIDByUnitName(gomock.Any(), unitName).Return(expectedAppID, nil)

	obtainedAppID, err := s.service.GetApplicationIDByUnitName(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedAppID, gc.DeepEquals, expectedAppID)
}

func (s *applicationServiceSuite) TestGetCharmModifiedVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetCharmModifiedVersion(gomock.Any(), appUUID).Return(42, nil)

	obtained, err := s.service.GetCharmModifiedVersion(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, gc.DeepEquals, 42)
}

func (s *applicationServiceSuite) TestGetAsyncCharmDownloadInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, nil)

	obtained, err := s.service.GetAsyncCharmDownloadInfo(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, gc.DeepEquals, info)
}

func (s *applicationServiceSuite) TestResolveCharmDownload(c *gc.C) {
	defer s.setupMocks(c).Finish()

	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")

	// This will be removed once we get the information from charmhub store.
	// For now, just brute force our way through to get the actions.
	ch := testcharms.Repo.CharmDir("dummy")
	actions, err := encodeActions(ch.Actions())
	c.Assert(err, jc.ErrorIsNil)

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash-256",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, nil)
	s.charmStore.EXPECT().Store(gomock.Any(), path, int64(42), "hash-384").Return(store.StoreResult{
		UniqueName:      "somepath",
		ObjectStoreUUID: objectStoreUUID,
	}, nil)
	s.state.EXPECT().ResolveCharmDownload(gomock.Any(), charmUUID, application.ResolvedCharmDownload{
		Actions:         actions,
		ObjectStoreUUID: objectStoreUUID,
		ArchivePath:     "somepath",
	})

	err = s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		SHA256:    "hash-256",
		SHA384:    "hash-384",
		Path:      path,
		Size:      42,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadInvalidApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.ResolveCharmDownload(context.Background(), "!!!!", application.ResolveCharmDownload{})
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadAlreadyAvailable(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, applicationerrors.CharmAlreadyAvailable)

	err := s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		Path:      "foo",
		Size:      42,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadAlreadyResolved(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, applicationerrors.CharmAlreadyResolved)

	err := s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		Path:      "foo",
		Size:      42,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadCharmUUIDMismatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: "blah",
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, nil)

	err := s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		Path:      path,
		Size:      42,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotResolved)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadNotStored(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash-256",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, nil)
	s.charmStore.EXPECT().Store(gomock.Any(), path, int64(42), "hash-384").Return(store.StoreResult{}, jujuerrors.NotFoundf("not found"))

	err := s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		SHA256:    "hash-256",
		SHA384:    "hash-384",
		Path:      path,
		Size:      42,
	})
	c.Assert(err, jc.ErrorIs, jujuerrors.NotFound)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadAlreadyStored(c *gc.C) {
	defer s.setupMocks(c).Finish()

	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")

	// This will be removed once we get the information from charmhub store.
	// For now, just brute force our way through to get the actions.
	ch := testcharms.Repo.CharmDir("dummy")
	actions, err := encodeActions(ch.Actions())
	c.Assert(err, jc.ErrorIsNil)

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash-256",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, nil)
	s.charmStore.EXPECT().Store(gomock.Any(), path, int64(42), "hash-384").Return(store.StoreResult{
		UniqueName:      "somepath",
		ObjectStoreUUID: objectStoreUUID,
	}, nil)
	s.state.EXPECT().ResolveCharmDownload(gomock.Any(), charmUUID, application.ResolvedCharmDownload{
		Actions:         actions,
		ObjectStoreUUID: objectStoreUUID,
		ArchivePath:     "somepath",
	})

	err = s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		SHA256:    "hash-256",
		SHA384:    "hash-384",
		Path:      path,
		Size:      42,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestGetApplicationsForRevisionUpdater(c *gc.C) {
	defer s.setupMocks(c).Finish()

	apps := []application.RevisionUpdaterApplication{
		{
			Name: "foo",
		},
	}

	s.state.EXPECT().GetApplicationsForRevisionUpdater(gomock.Any()).Return(apps, nil)

	results, err := s.service.GetApplicationsForRevisionUpdater(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, apps)
}

func (s *applicationServiceSuite) TestGetApplicationConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationConfig(gomock.Any(), appUUID).Return(map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, nil)

	results, err := s.service.GetApplicationConfig(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, config.ConfigAttributes{
		"foo": "bar",
	})
}

func (s *applicationServiceSuite) TestGetApplicationConfigWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationConfig(gomock.Any(), appUUID).Return(map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, errors.Errorf("boom"))

	_, err := s.service.GetApplicationConfig(context.Background(), appUUID)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *applicationServiceSuite) TestGetApplicationConfigNoConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationConfig(gomock.Any(), appUUID).Return(map[string]application.ApplicationConfig{}, nil)

	results, err := s.service.GetApplicationConfig(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, config.ConfigAttributes{})
}

func (s *applicationServiceSuite) TestGetApplicationConfigInvalidApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationConfig(context.Background(), "!!!")
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestGetApplicationTrustSetting(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationTrustSetting(gomock.Any(), appUUID).Return(true, nil)

	results, err := s.service.GetApplicationTrustSetting(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.IsTrue)
}

func (s *applicationServiceSuite) TestGetApplicationTrustSettingInvalidApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationTrustSetting(context.Background(), "!!!")
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestUnsetApplicationConfigKeys(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().UnsetApplicationConfigKeys(gomock.Any(), appUUID, []string{"a", "b"}).Return(nil)

	err := s.service.UnsetApplicationConfigKeys(context.Background(), appUUID, []string{"a", "b"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUnsetApplicationConfigKeysNoValues(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	err := s.service.UnsetApplicationConfigKeys(context.Background(), appUUID, []string{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUnsetApplicationConfigKeysInvalidApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.UnsetApplicationConfigKeys(context.Background(), "!!!", []string{"a", "b"})
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestSetApplicationConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(charmUUID, applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    applicationcharm.OptionString,
				Default: "baz",
			},
		},
	}, nil)
	s.state.EXPECT().SetApplicationConfigAndSettings(gomock.Any(), appUUID, charmUUID, map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, application.ApplicationSettings{
		Trust: true,
	}).Return(nil)

	err := s.service.SetApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestSetApplicationConfigNoCharmConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(
		charmUUID,
		applicationcharm.Config{},
		applicationerrors.CharmNotFound,
	)

	err := s.service.SetApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *applicationServiceSuite) TestSetApplicationConfigWithNoCharmConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(charmUUID, applicationcharm.Config{
		Options: map[string]applicationcharm.Option{},
	}, nil)

	err := s.service.SetApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidCharmConfig)
}

func (s *applicationServiceSuite) TestSetApplicationConfigInvalidOptionType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(charmUUID, applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    "blah",
				Default: "baz",
			},
		},
	}, nil)

	err := s.service.SetApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, gc.ErrorMatches, `.*unknown option type "blah"`)
}

func (s *applicationServiceSuite) TestSetApplicationConfigInvalidTrustType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(charmUUID, applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    "string",
				Default: "baz",
			},
		},
	}, nil)

	err := s.service.SetApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "FOO",
		"foo":   "bar",
	})
	c.Assert(err, gc.ErrorMatches, `.*parsing trust setting.*`)
}

func (s *applicationServiceSuite) TestSetApplicationConfigNoConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(charmUUID, applicationcharm.Config{}, nil)
	s.state.EXPECT().SetApplicationConfigAndSettings(
		gomock.Any(), appUUID, charmUUID,
		map[string]application.ApplicationConfig{},
		application.ApplicationSettings{},
	).Return(nil)

	err := s.service.SetApplicationConfig(context.Background(), appUUID, map[string]string{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestSetApplicationConfigInvalidApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetApplicationConfig(context.Background(), "!!!", nil)
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

type applicationWatcherServiceSuite struct {
	testing.IsolationSuite

	service *WatchableService

	state          *MockState
	charm          *MockCharm
	secret         *MockDeleteSecretState
	clock          *testclock.Clock
	watcherFactory *MockWatcherFactory
}

var _ = gc.Suite(&applicationWatcherServiceSuite{})

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapper(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	appID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), []coreapplication.ID{appID}).Return([]coreapplication.ID{
		appID,
	}, nil)

	changes := []changestream.ChangeEvent{&changeEvent{
		typ:       changestream.All,
		namespace: "application",
		changed:   appID.String(),
	}}

	result, err := s.service.watchApplicationsWithPendingCharmsMapper(context.Background(), changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, changes)
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapperInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	changes := []changestream.ChangeEvent{&changeEvent{
		typ:       changestream.All,
		namespace: "application",
		changed:   "foo",
	}}

	_, err := s.service.watchApplicationsWithPendingCharmsMapper(context.Background(), changes)
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapperOrder(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	appIDs := make([]coreapplication.ID, 4)
	for i := range appIDs {
		appIDs[i] = applicationtesting.GenApplicationUUID(c)
	}

	changes := make([]changestream.ChangeEvent, len(appIDs))
	for i, appID := range appIDs {
		changes[i] = &changeEvent{
			typ:       changestream.All,
			namespace: "application",
			changed:   appID.String(),
		}
	}

	// Ensure order is preserved if the state returns the uuids in an unexpected
	// order. This is because we can't guarantee the order if there are holes in
	// the pending sequence.

	shuffle := make([]coreapplication.ID, len(appIDs))
	copy(shuffle, appIDs)
	rand.Shuffle(len(shuffle), func(i, j int) {
		shuffle[i], shuffle[j] = shuffle[j], shuffle[i]
	})

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), appIDs).Return(shuffle, nil)

	result, err := s.service.watchApplicationsWithPendingCharmsMapper(context.Background(), changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, changes)
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapperDropped(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	appIDs := make([]coreapplication.ID, 10)
	for i := range appIDs {
		appIDs[i] = applicationtesting.GenApplicationUUID(c)
	}

	changes := make([]changestream.ChangeEvent, len(appIDs))
	for i, appID := range appIDs {
		changes[i] = &changeEvent{
			typ:       changestream.All,
			namespace: "application",
			changed:   appID.String(),
		}
	}

	// Ensure order is preserved if the state returns the uuids in an unexpected
	// order. This is because we can't guarantee the order if there are holes in
	// the pending sequence.

	var dropped []coreapplication.ID
	var expected []changestream.ChangeEvent
	for i, appID := range appIDs {
		if rand.IntN(2) == 0 {
			continue
		}
		dropped = append(dropped, appID)
		expected = append(expected, changes[i])
	}

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), appIDs).Return(dropped, nil)

	result, err := s.service.watchApplicationsWithPendingCharmsMapper(context.Background(), changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, expected)
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapperOrderDropped(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	appIDs := make([]coreapplication.ID, 10)
	for i := range appIDs {
		appIDs[i] = applicationtesting.GenApplicationUUID(c)
	}

	changes := make([]changestream.ChangeEvent, len(appIDs))
	for i, appID := range appIDs {
		changes[i] = &changeEvent{
			typ:       changestream.All,
			namespace: "application",
			changed:   appID.String(),
		}
	}

	// Ensure order is preserved if the state returns the uuids in an unexpected
	// order. This is because we can't guarantee the order if there are holes in
	// the pending sequence.

	var dropped []coreapplication.ID
	var expected []changestream.ChangeEvent
	for i, appID := range appIDs {
		if rand.IntN(2) == 0 {
			continue
		}
		dropped = append(dropped, appID)
		expected = append(expected, changes[i])
	}

	// Shuffle them to replicate out of order return.

	shuffle := make([]coreapplication.ID, len(dropped))
	copy(shuffle, dropped)
	rand.Shuffle(len(shuffle), func(i, j int) {
		shuffle[i], shuffle[j] = shuffle[j], shuffle[i]
	})

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), appIDs).Return(shuffle, nil)

	result, err := s.service.watchApplicationsWithPendingCharmsMapper(context.Background(), changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, expected)
}

func (s *applicationWatcherServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.secret = NewMockDeleteSecretState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	registry := corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	})

	modelUUID := modeltesting.GenModelUUID(c)

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewWatchableService(
		s.state,
		s.secret,
		registry,
		modelUUID,
		s.watcherFactory,
		nil,
		nil,
		nil,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
	s.service.clock = s.clock

	s.state.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(ctx domain.AtomicContext) error) error {
		return fn(domaintesting.NewAtomicContext(ctx))
	}).AnyTimes()

	return ctrl
}

type providerServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&providerServiceSuite{})

func (s *providerServiceSuite) TestGetSupportedFeatures(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentVersion := version.MustParse("4.0.0")
	s.agentVersionGetter.EXPECT().GetModelTargetAgentVersion(gomock.Any(), s.modelID).Return(agentVersion, nil)

	s.provider.EXPECT().SupportedFeatures().Return(assumes.FeatureSet{}, nil)

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
		return s.provider, jujuerrors.NotSupported
	})
	defer ctrl.Finish()

	agentVersion := version.MustParse("4.0.0")
	s.agentVersionGetter.EXPECT().GetModelTargetAgentVersion(gomock.Any(), s.modelID).Return(agentVersion, nil)

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
