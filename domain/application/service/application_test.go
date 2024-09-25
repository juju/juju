// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/assumes"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	jujutesting "github.com/juju/juju/internal/testing"
)

type applicationServiceSuite struct {
	testing.IsolationSuite

	state   *MockApplicationState
	charm   *MockCharm
	service *ApplicationService
}

var _ = gc.Suite(&applicationServiceSuite{})

func (s *applicationServiceSuite) TestCreateApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.UpsertUnitArg{
		UnitName: "ubuntu/666",
	}
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
		},
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
		Origin: domaincharm.CharmOrigin{
			ReferenceName: "ubuntu",
			Source:        domaincharm.CharmHubSource,
			Revision:      42,
		},
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "ubuntu", app, u).Return(id, nil)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).AnyTimes()

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
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

	_, err := s.service.CreateApplication(context.Background(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "666",
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

	s.charm.EXPECT().Meta().Return(nil).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmMetadataNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationWithNoArchitecture(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{Name: "foo"}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.Platform{Channel: "24.04", OS: "ubuntu"},
	}, AddApplicationArgs{
		ReferenceName: "foo",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmOriginNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	rErr := errors.New("boom")
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "foo", gomock.Any()).Return(id, rErr)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
	}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		ReferenceName: "foo",
	})
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `creating application "foo": boom`)
}

func (s *applicationServiceSuite) TestCreateWithStorageBlock(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.UpsertUnitArg{
		UnitName: "ubuntu/666",
	}
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]domaincharm.Storage{
				"data": {
					Name:        "data",
					Type:        domaincharm.StorageBlock,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
		Origin: domaincharm.CharmOrigin{
			ReferenceName: "foo",
			Source:        domaincharm.LocalSource,
			Revision:      42,
		},
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "foo", app, u).Return(id, nil)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
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
	}).AnyTimes()
	pool := domainstorage.StoragePoolDetails{Name: "loop", Provider: "loop"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "loop").Return(pool, nil)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.Local,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithStorageBlockDefaultSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.UpsertUnitArg{
		UnitName: "ubuntu/666",
	}
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]domaincharm.Storage{
				"data": {
					Name:        "data",
					Type:        domaincharm.StorageBlock,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
		Origin: domaincharm.CharmOrigin{
			ReferenceName: "foo",
			Source:        domaincharm.CharmHubSource,
			Revision:      42,
		},
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{DefaultBlockSource: ptr("fast")}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "foo", app, u).Return(id, nil)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
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
	}).AnyTimes()
	pool := domainstorage.StoragePoolDetails{Name: "fast", Provider: "modelscoped-block"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "fast").Return(pool, nil)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		Storage: map[string]storage.Directive{
			"data": {Count: 2},
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithStorageFilesystem(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.UpsertUnitArg{
		UnitName: "ubuntu/666",
	}
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]domaincharm.Storage{
				"data": {
					Name:        "data",
					Type:        domaincharm.StorageFilesystem,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
		Origin: domaincharm.CharmOrigin{
			ReferenceName: "foo",
			Source:        domaincharm.CharmHubSource,
			Revision:      42,
		},
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "foo", app, u).Return(id, nil)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
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
	}).AnyTimes()
	pool := domainstorage.StoragePoolDetails{Name: "rootfs", Provider: "rootfs"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "rootfs").Return(pool, nil)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithStorageFilesystemDefaultSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.UpsertUnitArg{
		UnitName: "ubuntu/666",
	}
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]domaincharm.Storage{
				"data": {
					Name:        "data",
					Type:        domaincharm.StorageFilesystem,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
		Origin: domaincharm.CharmOrigin{
			ReferenceName: "foo",
			Source:        domaincharm.CharmHubSource,
			Revision:      42,
		},
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{DefaultFilesystemSource: ptr("fast")}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "foo", app, u).Return(id, nil)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
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
	}).AnyTimes()
	pool := domainstorage.StoragePoolDetails{Name: "fast", Provider: "modelscoped"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "fast").Return(pool, nil)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		Storage: map[string]storage.Directive{
			"data": {Count: 2},
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithSharedStorageMissingDirectives(c *gc.C) {
	defer s.setupMocks(c).Finish()

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
	}).AnyTimes()

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		ReferenceName: "foo",
	}, a)
	c.Assert(err, jc.ErrorIs, storageerrors.MissingSharedStorageDirectiveError)
	c.Assert(err, gc.ErrorMatches, `adding default storage directives: no storage directive specified for shared charm storage "data"`)
}

func (s *applicationServiceSuite) TestCreateWithStorageValidates(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "loop").
		Return(domainstorage.StoragePoolDetails{}, storageerrors.PoolNotFoundError).MaxTimes(1)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "mine",
		Storage: map[string]charm.Storage{
			"data": {
				Name: "data",
				Type: charm.StorageBlock,
			},
		},
	}).AnyTimes()

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		Storage: map[string]storage.Directive{
			"logs": {Count: 2},
		},
	}, a)
	c.Assert(err, gc.ErrorMatches, `invalid storage directives: charm "mine" has no store called "logs"`)
}

func (s *applicationServiceSuite) TestAddUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.UpsertUnitArg{
		UnitName: "ubuntu/666",
	}
	s.state.EXPECT().AddUnits(gomock.Any(), "666", u).Return(nil)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), "666", a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestRegisterCAASUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).Return(nil)

	p := RegisterCAASUnitParams{
		UnitName:     "foo/666",
		PasswordHash: ptr("passwordhash"),
		ProviderId:   ptr("provider-id"),
		OrderedScale: true,
		OrderedId:    1,
	}
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIsNil)
}

var unitParams = RegisterCAASUnitParams{
	UnitName:     "foo/666",
	PasswordHash: ptr("passwordhash"),
	ProviderId:   ptr("provider-id"),
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
	p.ProviderId = nil
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "provider id not valid")
}

func (s *applicationServiceSuite) TestRegisterCAASUnitMissingPasswordHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.PasswordHash = nil
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "password hash not valid")
}

func (s *applicationServiceSuite) TestGetCharmByApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetCharmByApplicationName(gomock.Any(), "foo").Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name: "foo",

			// RunAs becomes mandatory when being persisted. Empty string is not
			// allowed.
			RunAs: "default",
		},
	}, domaincharm.CharmOrigin{
		ReferenceName: "bar",
		Revision:      42,
	}, application.Platform{
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.AMD64,
	}, nil)

	metadata, origin, platform, err := s.service.GetCharmByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata.Meta(), gc.DeepEquals, &charm.Meta{
		Name: "foo",

		// Notice that the RunAs field becomes empty string when being returned.
	})
	c.Check(origin, gc.DeepEquals, domaincharm.CharmOrigin{
		ReferenceName: "bar",
		Revision:      42,
	})
	c.Check(platform, gc.DeepEquals, application.Platform{
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.AMD64,
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

func (s *applicationServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockApplicationState(ctrl)
	s.charm = NewMockCharm(ctrl)
	registry := storage.ChainedProviderRegistry{
		dummystorage.StorageProviders(),
		provider.CommonStorageProviders(),
	}
	params := ApplicationServiceParams{
		StorageRegistry: registry,
		Secrets:         NotImplementedSecretService{},
	}
	s.service = NewApplicationService(s.state, params, loggertesting.WrapCheckLog(c))

	return ctrl
}

type providerApplicationServiceSuite struct {
	service *ProviderApplicationService

	modelID model.UUID

	agentVersionGetter *MockAgentVersionGetter
	provider           *MockProvider
}

var _ = gc.Suite(&providerApplicationServiceSuite{})

func (s *providerApplicationServiceSuite) TestGetSupportedFeatures(c *gc.C) {
	ctrl := s.setupMocks(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	})
	defer ctrl.Finish()

	agentVersion := version.MustParse("4.0.0")
	s.agentVersionGetter.EXPECT().GetModelAgentVersion(gomock.Any(), s.modelID).Return(agentVersion, nil)

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

func (s *providerApplicationServiceSuite) TestGetSupportedFeaturesNotSupported(c *gc.C) {
	ctrl := s.setupMocks(c, func(ctx context.Context) (Provider, error) {
		return s.provider, jujuerrors.NotSupported
	})
	defer ctrl.Finish()

	agentVersion := version.MustParse("4.0.0")
	s.agentVersionGetter.EXPECT().GetModelAgentVersion(gomock.Any(), s.modelID).Return(agentVersion, nil)

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

func (s *providerApplicationServiceSuite) setupMocks(c *gc.C, fn func(ctx context.Context) (Provider, error)) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelID = model.UUID(jujutesting.ModelTag.Id())

	s.agentVersionGetter = NewMockAgentVersionGetter(ctrl)
	s.provider = NewMockProvider(ctrl)

	s.service = &ProviderApplicationService{
		modelID:            s.modelID,
		agentVersionGetter: s.agentVersionGetter,
		provider:           fn,
	}

	return ctrl
}
