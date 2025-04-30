// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/devices"
	coreerrors "github.com/juju/juju/core/errors"
	modeltesting "github.com/juju/juju/core/model/testing"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/charm/store"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/internal/charm"
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

func (s *applicationServiceSuite) TestGetCharmLocatorByApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharmLocatorByCharmID(gomock.Any(), id).Return(applicationcharm.CharmLocator{
		Name:         "bar",
		Revision:     42,
		Source:       applicationcharm.LocalSource,
		Architecture: architecture.AMD64,
	}, nil)

	expectedLocator := applicationcharm.CharmLocator{
		Name:         "bar",
		Revision:     42,
		Source:       applicationcharm.LocalSource,
		Architecture: architecture.AMD64,
	}
	locator, err := s.service.GetCharmLocatorByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(locator, gc.DeepEquals, expectedLocator)
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
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
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
	s.charmStore.EXPECT().Store(gomock.Any(), path, int64(42), "hash-384").Return(store.StoreResult{}, errors.Errorf("not found %w", coreerrors.NotFound))

	err := s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		SHA256:    "hash-256",
		SHA384:    "hash-384",
		Path:      path,
		Size:      42,
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
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

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).Return(map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, application.ApplicationSettings{
		Trust: true,
	}, nil)

	results, err := s.service.GetApplicationConfig(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, config.ConfigAttributes{
		"foo":   "bar",
		"trust": true,
	})
}

func (s *applicationServiceSuite) TestGetApplicationConfigWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).Return(map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, application.ApplicationSettings{
		Trust: true,
	}, errors.Errorf("boom"))

	_, err := s.service.GetApplicationConfig(context.Background(), appUUID)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *applicationServiceSuite) TestGetApplicationConfigNoConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).
		Return(map[string]application.ApplicationConfig{}, application.ApplicationSettings{}, nil)

	results, err := s.service.GetApplicationConfig(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, config.ConfigAttributes{
		"trust": false,
	})
}

func (s *applicationServiceSuite) TestGetApplicationConfigNoConfigWithTrust(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).
		Return(map[string]application.ApplicationConfig{}, application.ApplicationSettings{
			Trust: true,
		}, nil)

	results, err := s.service.GetApplicationConfig(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, config.ConfigAttributes{
		"trust": true,
	})
}

func (s *applicationServiceSuite) TestGetApplicationConfigInvalidApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationConfig(context.Background(), "!!!")
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
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
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
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
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return("", applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    applicationcharm.OptionString,
				Default: "baz",
			},
		},
	}, nil)
	s.state.EXPECT().UpdateApplicationConfigAndSettings(gomock.Any(), appUUID, map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, application.UpdateApplicationSettingsArg{
		Trust: ptr(true),
	}).Return(nil)

	err := s.service.UpdateApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigRemoveTrust(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return("", applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    applicationcharm.OptionString,
				Default: "baz",
			},
		},
	}, nil)
	s.state.EXPECT().UpdateApplicationConfigAndSettings(gomock.Any(), appUUID, map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, application.UpdateApplicationSettingsArg{
		Trust: ptr(false),
	}).Return(nil)

	err := s.service.UpdateApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "false",
		"foo":   "bar",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigNoTrust(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return("", applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    applicationcharm.OptionString,
				Default: "baz",
			},
		},
	}, nil)
	s.state.EXPECT().UpdateApplicationConfigAndSettings(gomock.Any(), appUUID, map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, application.UpdateApplicationSettingsArg{}).Return(nil)

	err := s.service.UpdateApplicationConfig(context.Background(), appUUID, map[string]string{
		"foo": "bar",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigNoCharmConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(
		"",
		applicationcharm.Config{},
		applicationerrors.CharmNotFound,
	)

	err := s.service.UpdateApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigWithNoCharmConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return("", applicationcharm.Config{
		Options: map[string]applicationcharm.Option{},
	}, nil)

	err := s.service.UpdateApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidApplicationConfig)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigInvalidOptionType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return("", applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    "blah",
				Default: "baz",
			},
		},
	}, nil)

	err := s.service.UpdateApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, gc.ErrorMatches, `.*unknown option type "blah"`)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigInvalidTrustType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return("", applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    "string",
				Default: "baz",
			},
		},
	}, nil)

	err := s.service.UpdateApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "FOO",
		"foo":   "bar",
	})
	c.Assert(err, gc.ErrorMatches, `.*parsing trust setting.*`)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigNoConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return("", applicationcharm.Config{}, nil)
	s.state.EXPECT().UpdateApplicationConfigAndSettings(
		gomock.Any(), appUUID,
		map[string]application.ApplicationConfig{},
		application.UpdateApplicationSettingsArg{},
	).Return(nil)

	err := s.service.UpdateApplicationConfig(context.Background(), appUUID, map[string]string{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigInvalidApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.UpdateApplicationConfig(context.Background(), "!!!", nil)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *applicationServiceSuite) TestGetApplicationAndCharmConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	appConfig := map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}
	settings := application.ApplicationSettings{
		Trust: true,
	}
	charmConfig := applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    applicationcharm.OptionString,
				Default: "baz",
			},
		},
	}
	charmOrigin := application.CharmOrigin{
		Name:   "foo",
		Source: applicationcharm.CharmHubSource,
		Platform: deployment.Platform{
			Architecture: architecture.AMD64,
			Channel:      "stable",
			OSType:       deployment.Ubuntu,
		},
		Channel: &deployment.Channel{
			Risk: deployment.RiskStable,
		},
	}

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).Return(appConfig, settings, nil)
	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(charmUUID, charmConfig, nil)
	s.state.EXPECT().IsSubordinateCharm(gomock.Any(), charmUUID).Return(false, nil)
	s.state.EXPECT().GetApplicationCharmOrigin(gomock.Any(), appUUID).Return(charmOrigin, nil)

	results, err := s.service.GetApplicationAndCharmConfig(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, ApplicationConfig{
		ApplicationConfig: config.ConfigAttributes{
			"foo": "bar",
		},
		CharmConfig: charm.Config{
			Options: map[string]charm.Option{
				"foo": {
					Type:    "string",
					Default: "baz",
				},
			},
		},
		Trust:     true,
		Principal: true,
		CharmName: "foo",
		CharmOrigin: corecharm.Origin{
			Source: corecharm.CharmHub,
			Platform: corecharm.Platform{
				Architecture: arch.AMD64,
				Channel:      "stable",
				OS:           "Ubuntu",
			},
			Channel: &charm.Channel{
				Risk: charm.Stable,
			},
		},
	})
}

func (s *applicationServiceSuite) TestGetApplicationAndCharmConfigInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationAndCharmConfig(context.Background(), "!!!")
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *applicationServiceSuite) TestGetApplicationAndCharmConfigNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	appConfig := map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}
	settings := application.ApplicationSettings{
		Trust: true,
	}

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).Return(appConfig, settings, applicationerrors.ApplicationNotFound)

	_, err := s.service.GetApplicationAndCharmConfig(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationServiceSuite) TestDecodeCharmOrigin(c *gc.C) {
	origin := application.CharmOrigin{
		Name:   "foo",
		Source: applicationcharm.CharmHubSource,
		Platform: deployment.Platform{
			Architecture: architecture.AMD64,
			Channel:      "stable",
			OSType:       deployment.Ubuntu,
		},
		Channel: &deployment.Channel{
			Risk: deployment.RiskStable,
		},
	}

	decoded, err := decodeCharmOrigin(origin)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(decoded, gc.DeepEquals, corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Architecture: arch.AMD64,
			Channel:      "stable",
			OS:           "Ubuntu",
		},
		Channel: &charm.Channel{
			Risk: charm.Stable,
		},
	})
}

func (s *applicationServiceSuite) TestDecodeCharmSource(c *gc.C) {
	source := applicationcharm.CharmHubSource
	decoded, err := decodeCharmSource(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(decoded, gc.Equals, corecharm.CharmHub)

	source = applicationcharm.LocalSource
	decoded, err = decodeCharmSource(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(decoded, gc.Equals, corecharm.Local)

	_, err = decodeCharmSource("boom")
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *applicationServiceSuite) TestDecodePlatform(c *gc.C) {
	platform := deployment.Platform{
		Architecture: architecture.AMD64,
		Channel:      "stable",
		OSType:       deployment.Ubuntu,
	}

	decoded, err := decodePlatform(platform)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(decoded, gc.DeepEquals, corecharm.Platform{
		Architecture: arch.AMD64,
		Channel:      "stable",
		OS:           "Ubuntu",
	})
}

func (s *applicationServiceSuite) TestDecodePlatformArchError(c *gc.C) {
	platform := deployment.Platform{
		Architecture: 99,
		Channel:      "stable",
		OSType:       deployment.Ubuntu,
	}

	_, err := decodePlatform(platform)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *applicationServiceSuite) TestDecodePlatformOSError(c *gc.C) {
	platform := deployment.Platform{
		Architecture: architecture.AMD64,
		Channel:      "stable",
		OSType:       99,
	}

	_, err := decodePlatform(platform)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *applicationServiceSuite) TestDecodeChannelNilChannel(c *gc.C) {
	ch, err := decodeChannel(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch, gc.IsNil)
}

func (s *applicationServiceSuite) TestDecodeChannel(c *gc.C) {
	ch, err := decodeChannel(&deployment.Channel{
		Risk: deployment.RiskStable,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch, gc.DeepEquals, &charm.Channel{
		Risk: charm.Stable,
	})
}

func (s *applicationServiceSuite) TestDecodeChannelInvalidRisk(c *gc.C) {
	_, err := decodeChannel(&deployment.Channel{
		Risk: "risk",
	})
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *applicationServiceSuite) TestDecodeRisk(c *gc.C) {
	tests := []struct {
		risk     deployment.ChannelRisk
		expected charm.Risk
	}{
		{
			risk:     deployment.RiskStable,
			expected: charm.Stable,
		},
		{
			risk:     deployment.RiskCandidate,
			expected: charm.Candidate,
		},
		{
			risk:     deployment.RiskBeta,
			expected: charm.Beta,
		},
		{
			risk:     deployment.RiskEdge,
			expected: charm.Edge,
		},
	}
	for i, test := range tests {
		c.Logf("test %d", i)
		decoded, err := decodeRisk(test.risk)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(decoded, gc.Equals, test.expected)
	}
}

func (s *applicationServiceSuite) TestGetDeviceConstraintsAppNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "unknown-app").Return("", errors.Errorf("%w", applicationerrors.ApplicationNotFound))

	_, err := s.service.GetDeviceConstraints(context.Background(), "unknown-app")
	c.Assert(err, gc.ErrorMatches, applicationerrors.ApplicationNotFound.Error())
}

func (s *applicationServiceSuite) TestGetDeviceConstraintsDeadApp(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "dead-app").Return(coreapplication.ID("foo"), nil)
	s.state.EXPECT().GetDeviceConstraints(gomock.Any(), coreapplication.ID("foo")).Return(nil, errors.Errorf("%w", applicationerrors.ApplicationIsDead))

	_, err := s.service.GetDeviceConstraints(context.Background(), "dead-app")
	c.Assert(err, gc.ErrorMatches, applicationerrors.ApplicationIsDead.Error())
}

func (s *applicationServiceSuite) TestGetDeviceConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(coreapplication.ID("foo-uuid"), nil)
	s.state.EXPECT().GetDeviceConstraints(gomock.Any(), coreapplication.ID("foo-uuid")).Return(map[string]devices.Constraints{
		"dev0": {
			Type:  "type0",
			Count: 42,
		},
	}, nil)

	cons, err := s.service.GetDeviceConstraints(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, gc.DeepEquals, map[string]devices.Constraints{
		"dev0": {
			Type:  "type0",
			Count: 42,
		},
	})
}

type applicationWatcherServiceSuite struct {
	testing.IsolationSuite

	service *WatchableService

	state          *MockState
	charm          *MockCharm
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
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
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
		domaintesting.NoopLeaderEnsurer(),
		registry,
		modelUUID,
		s.watcherFactory,
		nil,
		nil,
		nil,
		nil,
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
	s.service.clock = s.clock

	return ctrl
}
