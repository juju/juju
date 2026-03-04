// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/devices"
	coreerrors "github.com/juju/juju/core/errors"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/core/os/ostype"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/charm/store"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/application/service/storage"
	"github.com/juju/juju/domain/deployment"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/testcharms"
)

type applicationServiceSuite struct {
	baseSuite
}

func TestApplicationServiceSuite(t *testing.T) {
	tc.Run(t, &applicationServiceSuite{})
}

func (s *applicationServiceSuite) TestGetCharmByApplicationID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), id).Return(applicationcharm.Charm{
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

	ch, locator, err := s.service.GetCharmByApplicationUUID(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch.Meta(), tc.DeepEquals, &internalcharm.Meta{
		Name: "foo",

		// Notice that the RunAs field becomes empty string when being returned.
	})
	c.Check(locator, tc.DeepEquals, applicationcharm.CharmLocator{
		Name:         "bar",
		Revision:     42,
		Source:       applicationcharm.LocalSource,
		Architecture: architecture.AMD64,
	})
}

func (s *applicationServiceSuite) TestGetApplicationName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationName(gomock.Any(), id).Return("foo", nil)

	name, err := s.service.GetApplicationName(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(name, tc.Equals, "foo")
}

func (s *applicationServiceSuite) TestGetApplicationNameNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationName(gomock.Any(), id).Return("", applicationerrors.ApplicationNotFound)

	_, err := s.service.GetApplicationName(c.Context(), id)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationServiceSuite) TestGetApplicationUUIDByName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return(id, nil)

	obtainedID, err := s.service.GetApplicationUUIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedID, tc.Equals, id)
}

func (s *applicationServiceSuite) TestGetApplicationUUIDByNameNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNotFound)

	_, err := s.service.GetApplicationUUIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationServiceSuite) TestGetApplicationDetailsByName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := tc.Must(c, coreapplication.NewUUID)
	details := application.ApplicationDetails{
		UUID:                   id,
		Life:                   life.Alive,
		Name:                   "foo",
		IsApplicationSynthetic: false,
	}

	s.state.EXPECT().GetApplicationDetailsByName(gomock.Any(), "foo").Return(details, nil)

	obtainedDetails, err := s.service.GetApplicationDetailsByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedDetails.UUID, tc.Equals, id)
	c.Check(obtainedDetails.Name, tc.Equals, "foo")
	c.Check(obtainedDetails.Life, tc.Equals, life.Alive)
	c.Check(obtainedDetails.IsApplicationSynthetic, tc.Equals, false)
}

func (s *applicationServiceSuite) TestGetApplicationDetailsByNameNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationDetailsByName(gomock.Any(), "foo").Return(application.ApplicationDetails{}, applicationerrors.ApplicationNotFound)

	_, err := s.service.GetApplicationDetailsByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationServiceSuite) TestGetApplicationDetailsByNameInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationDetailsByName(c.Context(), "")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *applicationServiceSuite) TestGetCharmLocatorByApplicationName(c *tc.C) {
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
	locator, err := s.service.GetCharmLocatorByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(locator, tc.DeepEquals, expectedLocator)
}

func (s *applicationServiceSuite) TestGetApplicationUUIDByUnitName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedAppID := tc.Must(c, coreapplication.NewUUID)
	unitName := coreunit.Name("foo")
	s.state.EXPECT().GetApplicationUUIDByUnitName(gomock.Any(), unitName).Return(expectedAppID, nil)

	obtainedAppID, err := s.service.GetApplicationUUIDByUnitName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedAppID, tc.DeepEquals, expectedAppID)
}

func (s *applicationServiceSuite) TestGetCharmModifiedVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetCharmModifiedVersion(gomock.Any(), appUUID).Return(42, nil)

	obtained, err := s.service.GetCharmModifiedVersion(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, 42)
}

func (s *applicationServiceSuite) TestGetAsyncCharmDownloadInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
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

	obtained, err := s.service.GetAsyncCharmDownloadInfo(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, info)
}

func (s *applicationServiceSuite) TestResolveCharmDownload(c *tc.C) {
	defer s.setupMocks(c).Finish()

	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")

	// This will be removed once we get the information from charmhub store.
	// For now, just brute force our way through to get the actions.
	ch := testcharms.Repo.CharmDir("dummy")
	actions, err := encodeActions(ch.Actions())
	c.Assert(err, tc.ErrorIsNil)

	appUUID := tc.Must(c, coreapplication.NewUUID)
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

	err = s.service.ResolveCharmDownload(c.Context(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		SHA256:    "hash-256",
		SHA384:    "hash-384",
		Path:      path,
		Size:      42,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadInvalidApplicationID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.ResolveCharmDownload(c.Context(), "!!!!", application.ResolveCharmDownload{})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadAlreadyAvailable(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
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

	err := s.service.ResolveCharmDownload(c.Context(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		Path:      "foo",
		Size:      42,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadAlreadyResolved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
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

	err := s.service.ResolveCharmDownload(c.Context(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		Path:      "foo",
		Size:      42,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadCharmUUIDMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")

	appUUID := tc.Must(c, coreapplication.NewUUID)
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

	err := s.service.ResolveCharmDownload(c.Context(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		Path:      path,
		Size:      42,
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmNotResolved)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadNotStored(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")

	appUUID := tc.Must(c, coreapplication.NewUUID)
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

	err := s.service.ResolveCharmDownload(c.Context(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		SHA256:    "hash-256",
		SHA384:    "hash-384",
		Path:      path,
		Size:      42,
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadAlreadyStored(c *tc.C) {
	defer s.setupMocks(c).Finish()

	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")

	// This will be removed once we get the information from charmhub store.
	// For now, just brute force our way through to get the actions.
	ch := testcharms.Repo.CharmDir("dummy")
	actions, err := encodeActions(ch.Actions())
	c.Assert(err, tc.ErrorIsNil)

	appUUID := tc.Must(c, coreapplication.NewUUID)
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

	err = s.service.ResolveCharmDownload(c.Context(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		SHA256:    "hash-256",
		SHA384:    "hash-384",
		Path:      path,
		Size:      42,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestGetApplicationsForRevisionUpdater(c *tc.C) {
	defer s.setupMocks(c).Finish()

	apps := []application.RevisionUpdaterApplication{
		{
			Name: "foo",
		},
	}

	s.state.EXPECT().GetApplicationsForRevisionUpdater(gomock.Any()).Return(apps, nil)

	results, err := s.service.GetApplicationsForRevisionUpdater(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, apps)
}

func (s *applicationServiceSuite) TestGetApplicationConfigWithDefaults(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationConfigWithDefaults(gomock.Any(), appUUID).Return(map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: ptr("bar"),
		},
		"baz": {
			Type:  applicationcharm.OptionInt,
			Value: ptr("42"),
		},
		"qux": {
			Type: applicationcharm.OptionBool,
		},
	}, nil)

	results, err := s.service.GetApplicationConfigWithDefaults(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, internalcharm.Config{
		"foo": "bar",
		"baz": 42,
		"qux": nil,
	})
}

func (s *applicationServiceSuite) TestGetApplicationConfigWithErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationConfigWithDefaults(gomock.Any(), appUUID).Return(map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionBool,
			Value: ptr("not-a-parsable-bool"),
		},
	}, nil)

	_, err := s.service.GetApplicationConfigWithDefaults(c.Context(), appUUID)
	c.Assert(err, tc.ErrorMatches, ".*cannot convert string \"not-a-parsable-bool\" to bool.*")
}

func (s *applicationServiceSuite) TestGetApplicationConfigWithDefaultsWithError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationConfigWithDefaults(gomock.Any(), appUUID).Return(map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: ptr("bar"),
		},
	}, errors.Errorf("boom"))

	_, err := s.service.GetApplicationConfigWithDefaults(c.Context(), appUUID)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *applicationServiceSuite) TestGetApplicationConfigWithDefaultsNoConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationConfigWithDefaults(gomock.Any(), appUUID).
		Return(map[string]application.ApplicationConfig{}, nil)

	results, err := s.service.GetApplicationConfigWithDefaults(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.HasLen, 0)
}

func (s *applicationServiceSuite) TestGetApplicationTrustSetting(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	s.state.EXPECT().GetApplicationTrustSetting(gomock.Any(), appUUID).Return(true, nil)

	results, err := s.service.GetApplicationTrustSetting(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.IsTrue)
}

func (s *applicationServiceSuite) TestGetApplicationTrustSettingNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNotFound)

	_, err := s.service.GetApplicationTrustSetting(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationServiceSuite) TestGetApplicationCharmOrigin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetApplicationCharmOrigin(gomock.Any(), id).Return(application.CharmOrigin{
		Name:   "foo",
		Source: applicationcharm.CharmHubSource,
		Platform: deployment.Platform{
			Architecture: architecture.AMD64,
			Channel:      "stable",
			OSType:       deployment.Ubuntu,
		},
		Channel: &deployment.Channel{
			Track: "1.0",
			Risk:  "stable",
		},
		Revision:           42,
		Hash:               "hash",
		CharmhubIdentifier: "id",
	}, nil)

	origin, err := s.service.GetApplicationCharmOrigin(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(origin, tc.DeepEquals, corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Architecture: arch.AMD64,
			Channel:      "stable",
			OS:           ostype.Ubuntu.String(),
		},
		Channel: &internalcharm.Channel{
			Track: "1.0",
			Risk:  "stable",
		},
		Revision: ptr(42),
		Hash:     "hash",
		ID:       "id",
	})
}

func (s *applicationServiceSuite) TestGetApplicationCharmOriginGetApplicationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return(id, errors.Errorf("boom"))

	_, err := s.service.GetApplicationCharmOrigin(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *applicationServiceSuite) TestGetApplicationCharmOriginInvalidID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationCharmOrigin(c.Context(), "!!!!!!!!!!!")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *applicationServiceSuite) TestUnsetApplicationConfigKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().UnsetApplicationConfigKeys(gomock.Any(), appUUID, []string{"a", "b"}).Return(nil)

	err := s.service.UnsetApplicationConfigKeys(c.Context(), appUUID, []string{"a", "b"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUnsetApplicationConfigKeysNoValues(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	err := s.service.UnsetApplicationConfigKeys(c.Context(), appUUID, []string{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUnsetApplicationConfigKeysInvalidApplicationID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.UnsetApplicationConfigKeys(c.Context(), "!!!", []string{"a", "b"})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetCharmConfigByApplicationUUID(gomock.Any(), appUUID).Return("", applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    applicationcharm.OptionString,
				Default: "baz",
			},
		},
	}, nil)
	s.state.EXPECT().UpdateApplicationConfigAndSettings(gomock.Any(), appUUID, map[string]application.AddApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, application.UpdateApplicationSettingsArg{
		Trust: ptr(true),
	}).Return(nil)

	err := s.service.UpdateApplicationConfig(c.Context(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigRemoveTrust(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetCharmConfigByApplicationUUID(gomock.Any(), appUUID).Return("", applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    applicationcharm.OptionString,
				Default: "baz",
			},
		},
	}, nil)
	s.state.EXPECT().UpdateApplicationConfigAndSettings(gomock.Any(), appUUID, map[string]application.AddApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, application.UpdateApplicationSettingsArg{
		Trust: ptr(false),
	}).Return(nil)

	err := s.service.UpdateApplicationConfig(c.Context(), appUUID, map[string]string{
		"trust": "false",
		"foo":   "bar",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigNoTrust(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetCharmConfigByApplicationUUID(gomock.Any(), appUUID).Return("", applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    applicationcharm.OptionString,
				Default: "baz",
			},
		},
	}, nil)
	s.state.EXPECT().UpdateApplicationConfigAndSettings(gomock.Any(), appUUID, map[string]application.AddApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, application.UpdateApplicationSettingsArg{}).Return(nil)

	err := s.service.UpdateApplicationConfig(c.Context(), appUUID, map[string]string{
		"foo": "bar",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigNoCharmConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetCharmConfigByApplicationUUID(gomock.Any(), appUUID).Return(
		"",
		applicationcharm.Config{},
		applicationerrors.CharmNotFound,
	)

	err := s.service.UpdateApplicationConfig(c.Context(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigWithNoCharmConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetCharmConfigByApplicationUUID(gomock.Any(), appUUID).Return("", applicationcharm.Config{
		Options: map[string]applicationcharm.Option{},
	}, nil)

	err := s.service.UpdateApplicationConfig(c.Context(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.InvalidApplicationConfig)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigInvalidOptionType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetCharmConfigByApplicationUUID(gomock.Any(), appUUID).Return("", applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    "blah",
				Default: "baz",
			},
		},
	}, nil)

	err := s.service.UpdateApplicationConfig(c.Context(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown option type "blah"`)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigInvalidTrustType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetCharmConfigByApplicationUUID(gomock.Any(), appUUID).Return("", applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    "string",
				Default: "baz",
			},
		},
	}, nil)

	err := s.service.UpdateApplicationConfig(c.Context(), appUUID, map[string]string{
		"trust": "FOO",
		"foo":   "bar",
	})
	c.Assert(err, tc.ErrorMatches, `.*parsing trust setting.*`)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigNoConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetCharmConfigByApplicationUUID(gomock.Any(), appUUID).Return("", applicationcharm.Config{}, nil)
	s.state.EXPECT().UpdateApplicationConfigAndSettings(
		gomock.Any(), appUUID,
		nil,
		application.UpdateApplicationSettingsArg{},
	).Return(nil)

	err := s.service.UpdateApplicationConfig(c.Context(), appUUID, map[string]string{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUpdateApplicationConfigInvalidApplicationID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.UpdateApplicationConfig(c.Context(), "!!!", nil)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *applicationServiceSuite) TestGetApplicationAndCharmConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := charmtesting.GenCharmID(c)

	appConfig := map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: ptr("bar"),
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
		Name:     "foo",
		Source:   applicationcharm.CharmHubSource,
		Revision: 42,
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
	s.state.EXPECT().GetCharmConfigByApplicationUUID(gomock.Any(), appUUID).Return(charmUUID, charmConfig, nil)
	s.state.EXPECT().IsSubordinateCharm(gomock.Any(), charmUUID).Return(false, nil)
	s.state.EXPECT().GetApplicationCharmOrigin(gomock.Any(), appUUID).Return(charmOrigin, nil)

	results, err := s.service.GetApplicationAndCharmConfig(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, ApplicationConfig{
		ApplicationConfig: internalcharm.Config{
			"foo": "bar",
		},
		CharmConfig: internalcharm.ConfigSpec{
			Options: map[string]internalcharm.Option{
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
			Source:   corecharm.CharmHub,
			Revision: ptr(42),
			Platform: corecharm.Platform{
				Architecture: arch.AMD64,
				Channel:      "stable",
				OS:           "Ubuntu",
			},
			Channel: &internalcharm.Channel{
				Risk: internalcharm.Stable,
			},
		},
	})
}

func (s *applicationServiceSuite) TestGetApplicationAndCharmConfigInvalidID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationAndCharmConfig(c.Context(), "!!!")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *applicationServiceSuite) TestGetApplicationAndCharmConfigNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	appConfig := map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: ptr("bar"),
		},
	}
	settings := application.ApplicationSettings{
		Trust: true,
	}

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).Return(appConfig, settings, applicationerrors.ApplicationNotFound)

	_, err := s.service.GetApplicationAndCharmConfig(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationServiceSuite) TestDecodeCharmOrigin(c *tc.C) {
	origin := application.CharmOrigin{
		Name:               "foo",
		Source:             applicationcharm.CharmHubSource,
		Hash:               "hash",
		CharmhubIdentifier: "id",
		Revision:           42,
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
	c.Assert(err, tc.ErrorIsNil)

	c.Check(decoded, tc.DeepEquals, corecharm.Origin{
		Source:   corecharm.CharmHub,
		ID:       "id",
		Hash:     "hash",
		Revision: ptr(42),
		Platform: corecharm.Platform{
			Architecture: arch.AMD64,
			Channel:      "stable",
			OS:           "Ubuntu",
		},
		Channel: &internalcharm.Channel{
			Risk: internalcharm.Stable,
		},
	})
}

func (s *applicationServiceSuite) TestDecodeCharmOriginNegativeRevision(c *tc.C) {
	origin := application.CharmOrigin{
		Name:               "foo",
		Source:             applicationcharm.CharmHubSource,
		Hash:               "hash",
		CharmhubIdentifier: "id",
		Revision:           -1,
		Platform: deployment.Platform{
			Architecture: architecture.AMD64,
			Channel:      "stable",
			OSType:       deployment.Ubuntu,
		},
		Channel: &deployment.Channel{
			Track: "1.0",
			Risk:  "stable",
		},
	}

	decoded, err := decodeCharmOrigin(origin)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(decoded, tc.DeepEquals, corecharm.Origin{
		Source: corecharm.CharmHub,
		ID:     "id",
		Hash:   "hash",
		Platform: corecharm.Platform{
			Architecture: arch.AMD64,
			Channel:      "stable",
			OS:           ostype.Ubuntu.String(),
		},
		Channel: &internalcharm.Channel{
			Track: "1.0",
			Risk:  "stable",
		},
	})
}

func (s *applicationServiceSuite) TestDecodeCharmSource(c *tc.C) {
	source := applicationcharm.CharmHubSource
	decoded, err := decodeCharmSource(source)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(decoded, tc.Equals, corecharm.CharmHub)

	source = applicationcharm.LocalSource
	decoded, err = decodeCharmSource(source)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(decoded, tc.Equals, corecharm.Local)

	_, err = decodeCharmSource("boom")
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *applicationServiceSuite) TestDecodePlatform(c *tc.C) {
	platform := deployment.Platform{
		Architecture: architecture.AMD64,
		Channel:      "stable",
		OSType:       deployment.Ubuntu,
	}

	decoded, err := decodePlatform(platform)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(decoded, tc.DeepEquals, corecharm.Platform{
		Architecture: arch.AMD64,
		Channel:      "stable",
		OS:           "Ubuntu",
	})
}

func (s *applicationServiceSuite) TestDecodePlatformArchError(c *tc.C) {
	platform := deployment.Platform{
		Architecture: 99,
		Channel:      "stable",
		OSType:       deployment.Ubuntu,
	}

	_, err := decodePlatform(platform)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *applicationServiceSuite) TestDecodePlatformOSError(c *tc.C) {
	platform := deployment.Platform{
		Architecture: architecture.AMD64,
		Channel:      "stable",
		OSType:       99,
	}

	_, err := decodePlatform(platform)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *applicationServiceSuite) TestDecodeChannelNilChannel(c *tc.C) {
	ch, err := decodeChannel(nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch, tc.IsNil)
}

func (s *applicationServiceSuite) TestDecodeChannel(c *tc.C) {
	ch, err := decodeChannel(&deployment.Channel{
		Risk: deployment.RiskStable,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch, tc.DeepEquals, &internalcharm.Channel{
		Risk: internalcharm.Stable,
	})
}

func (s *applicationServiceSuite) TestDecodeChannelInvalidRisk(c *tc.C) {
	_, err := decodeChannel(&deployment.Channel{
		Risk: "risk",
	})
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *applicationServiceSuite) TestDecodeRisk(c *tc.C) {
	tests := []struct {
		risk     deployment.ChannelRisk
		expected internalcharm.Risk
	}{
		{
			risk:     deployment.RiskStable,
			expected: internalcharm.Stable,
		},
		{
			risk:     deployment.RiskCandidate,
			expected: internalcharm.Candidate,
		},
		{
			risk:     deployment.RiskBeta,
			expected: internalcharm.Beta,
		},
		{
			risk:     deployment.RiskEdge,
			expected: internalcharm.Edge,
		},
	}
	for i, test := range tests {
		c.Logf("test %d", i)
		decoded, err := decodeRisk(test.risk)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(decoded, tc.Equals, test.expected)
	}
}

func (s *applicationServiceSuite) TestGetAllEndpointBindings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllEndpointBindings(gomock.Any()).Return(map[string]map[string]string{
		"foo": {"bar": "baz"},
		"bar": {"baz": "qux"},
	}, nil)

	result, err := s.service.GetAllEndpointBindings(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]map[string]network.SpaceName{
		"foo": {"bar": "baz"},
		"bar": {"baz": "qux"},
	})
}

func (s *applicationServiceSuite) TestGetAllEndpointBindingsErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllEndpointBindings(gomock.Any()).Return(nil, errors.Errorf("boom"))

	_, err := s.service.GetAllEndpointBindings(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *applicationServiceSuite) TestGetApplicationEndpointBindingsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNotFound)

	_, err := s.service.GetApplicationEndpointBindings(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationServiceSuite) TestGetApplicationEndpointBindings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	s.state.EXPECT().GetApplicationEndpointBindings(gomock.Any(), appUUID).Return(map[string]string{
		"foo": "bar",
	}, nil)

	result, err := s.service.GetApplicationEndpointBindings(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]network.SpaceUUID{
		"foo": "bar",
	})
}

func (s *applicationServiceSuite) TestGetApplicationsBoundToSpace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	spaceUUID := networktesting.GenSpaceUUID(c)
	s.state.EXPECT().GetApplicationsBoundToSpace(gomock.Any(), spaceUUID.String()).Return([]string{"foo", "bar"}, nil)

	apps, err := s.service.GetApplicationsBoundToSpace(c.Context(), spaceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(apps, tc.SameContents, []string{"foo", "bar"})
}

func (s *applicationServiceSuite) TestGetApplicationsBoundToSpaceErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()

	spaceUUID := networktesting.GenSpaceUUID(c)
	s.state.EXPECT().GetApplicationsBoundToSpace(gomock.Any(), spaceUUID.String()).Return(nil, errors.Errorf("boom"))

	_, err := s.service.GetApplicationsBoundToSpace(c.Context(), spaceUUID)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *applicationServiceSuite) TestGetApplicationEndpointNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetApplicationEndpointNames(gomock.Any(), appUUID).Return([]string{"foo", "bar"}, nil)

	eps, err := s.service.GetApplicationEndpointNames(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(eps, tc.SameContents, []string{"foo", "bar"})
}

func (s *applicationServiceSuite) TestGetApplicationEndpointNamesAppNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetApplicationEndpointNames(gomock.Any(), appUUID).Return(nil, applicationerrors.ApplicationNotFound)

	_, err := s.service.GetApplicationEndpointNames(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationServiceSuite) TestMergeApplicationEndpointBindings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	bindings := map[string]network.SpaceName{
		"foo": "alpha",
		"bar": "beta",
	}
	expectedBindings := transform.Map(bindings, func(k string, v network.SpaceName) (string, string) {
		return k, v.String()
	})

	s.state.EXPECT().MergeApplicationEndpointBindings(gomock.Any(), appUUID.String(), expectedBindings, false).Return(nil)

	err := s.service.MergeApplicationEndpointBindings(c.Context(), appUUID, bindings, false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestMergeApplicationEndpointBindingsForce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	bindings := map[string]network.SpaceName{
		"foo": "alpha",
		"bar": "beta",
	}
	expectedBindings := transform.Map(bindings, func(k string, v network.SpaceName) (string, string) {
		return k, v.String()
	})

	s.state.EXPECT().MergeApplicationEndpointBindings(gomock.Any(), appUUID.String(), expectedBindings, true).Return(nil)

	err := s.service.MergeApplicationEndpointBindings(c.Context(), appUUID, bindings, true)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestMergeApplicationEndpointBindingsInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	bindings := map[string]network.SpaceName{
		"foo": "alpha",
		"bar": "beta",
	}
	expectedBindings := transform.Map(bindings, func(k string, v network.SpaceName) (string, string) {
		return k, v.String()
	})

	s.state.EXPECT().MergeApplicationEndpointBindings(gomock.Any(), appUUID.String(), expectedBindings, false).Return(errors.Errorf("boom"))

	err := s.service.MergeApplicationEndpointBindings(c.Context(), appUUID, bindings, false)
	c.Assert(err, tc.ErrorMatches, ".*boom.*")
}

func (s *applicationServiceSuite) TestGetDeviceConstraintsAppNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "unknown-app").Return("", errors.Errorf("%w", applicationerrors.ApplicationNotFound))

	_, err := s.service.GetDeviceConstraints(c.Context(), "unknown-app")
	c.Assert(err, tc.ErrorMatches, applicationerrors.ApplicationNotFound.Error())
}

func (s *applicationServiceSuite) TestGetDeviceConstraintsDeadApp(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "dead-app").Return(coreapplication.UUID("foo"), nil)
	s.state.EXPECT().GetDeviceConstraints(gomock.Any(), coreapplication.UUID("foo")).Return(nil, errors.Errorf("%w", applicationerrors.ApplicationIsDead))

	_, err := s.service.GetDeviceConstraints(c.Context(), "dead-app")
	c.Assert(err, tc.ErrorMatches, applicationerrors.ApplicationIsDead.Error())
}

func (s *applicationServiceSuite) TestGetDeviceConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return(coreapplication.UUID("foo-uuid"), nil)
	s.state.EXPECT().GetDeviceConstraints(gomock.Any(), coreapplication.UUID("foo-uuid")).Return(map[string]devices.Constraints{
		"dev0": {
			Type:  "type0",
			Count: 42,
		},
	}, nil)

	cons, err := s.service.GetDeviceConstraints(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, map[string]devices.Constraints{
		"dev0": {
			Type:  "type0",
			Count: 42,
		},
	})
}

func (s *applicationServiceSuite) TestIsControllerApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().IsControllerApplication(gomock.Any(), id).Return(false, nil)
	isController, err := s.service.IsControllerApplication(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isController, tc.IsFalse)

	s.state.EXPECT().IsControllerApplication(gomock.Any(), id).Return(true, nil)
	isController, err = s.service.IsControllerApplication(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isController, tc.IsTrue)
}

func (s *applicationWatcherServiceSuite) TestGetMachinesForApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	s.state.EXPECT().GetMachinesForApplication(gomock.Any(), appUUID.String()).Return([]string{"0", "1"}, nil)

	results, err := s.service.GetMachinesForApplication(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, []machine.Name{"0", "1"})
}

type applicationWatcherServiceSuite struct {
	testhelpers.IsolationSuite

	service *WatchableService

	state          *MockState
	storageService *MockStorageService
	charm          *MockCharm
	clock          *testclock.Clock
	watcherFactory *MockWatcherFactory
}

func TestApplicationWatcherServiceSuite(t *testing.T) {
	tc.Run(t, &applicationWatcherServiceSuite{})
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapper(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), []coreapplication.UUID{appUUID}).Return([]coreapplication.UUID{
		appUUID,
	}, nil)

	changes := []changestream.ChangeEvent{&changeEvent{
		typ:       changestream.All,
		namespace: "application",
		changed:   appUUID.String(),
	}}

	result, err := s.service.watchApplicationsWithPendingCharmsMapper(c.Context(), changes)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []string{appUUID.String()})
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapperInvalidID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	changes := []changestream.ChangeEvent{&changeEvent{
		typ:       changestream.All,
		namespace: "application",
		changed:   "foo",
	}}

	_, err := s.service.watchApplicationsWithPendingCharmsMapper(c.Context(), changes)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapperOrder(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	appUUIDs := make([]coreapplication.UUID, 4)
	for i := range appUUIDs {
		appUUIDs[i] = tc.Must(c, coreapplication.NewUUID)
	}

	changes := make([]changestream.ChangeEvent, len(appUUIDs))
	for i, appUUID := range appUUIDs {
		changes[i] = &changeEvent{
			typ:       changestream.All,
			namespace: "application",
			changed:   appUUID.String(),
		}
	}

	// Ensure order is preserved if the state returns the uuids in an unexpected
	// order. This is because we can't guarantee the order if there are holes in
	// the pending sequence.

	shuffle := make([]coreapplication.UUID, len(appUUIDs))
	copy(shuffle, appUUIDs)
	rand.Shuffle(len(shuffle), func(i, j int) {
		shuffle[i], shuffle[j] = shuffle[j], shuffle[i]
	})

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), appUUIDs).Return(shuffle, nil)

	result, err := s.service.watchApplicationsWithPendingCharmsMapper(c.Context(), changes)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, transform.Slice(changes, func(change changestream.ChangeEvent) string {
		return change.Changed()
	}))
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapperDropped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	appUUIDs := make([]coreapplication.UUID, 10)
	for i := range appUUIDs {
		appUUIDs[i] = tc.Must(c, coreapplication.NewUUID)
	}

	changes := make([]changestream.ChangeEvent, len(appUUIDs))
	for i, appUUID := range appUUIDs {
		changes[i] = &changeEvent{
			typ:       changestream.All,
			namespace: "application",
			changed:   appUUID.String(),
		}
	}

	// Ensure order is preserved if the state returns the uuids in an unexpected
	// order. This is because we can't guarantee the order if there are holes in
	// the pending sequence.

	var dropped []coreapplication.UUID
	var expected []string
	for i, appUUID := range appUUIDs {
		if rand.IntN(2) == 0 {
			continue
		}
		dropped = append(dropped, appUUID)
		expected = append(expected, changes[i].Changed())
	}

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), appUUIDs).Return(dropped, nil)

	result, err := s.service.watchApplicationsWithPendingCharmsMapper(c.Context(), changes)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, expected)
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapperOrderDropped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	appUUIDs := make([]coreapplication.UUID, 10)
	for i := range appUUIDs {
		appUUIDs[i] = tc.Must(c, coreapplication.NewUUID)
	}

	changes := make([]changestream.ChangeEvent, len(appUUIDs))
	for i, appUUID := range appUUIDs {
		changes[i] = &changeEvent{
			typ:       changestream.All,
			namespace: "application",
			changed:   appUUID.String(),
		}
	}

	// Ensure order is preserved if the state returns the uuids in an unexpected
	// order. This is because we can't guarantee the order if there are holes in
	// the pending sequence.

	var dropped []coreapplication.UUID
	var expected []string
	for i, appUUID := range appUUIDs {
		if rand.IntN(2) == 0 {
			continue
		}
		dropped = append(dropped, appUUID)
		expected = append(expected, changes[i].Changed())
	}

	// Shuffle them to replicate out of order return.

	shuffle := make([]coreapplication.UUID, len(dropped))
	copy(shuffle, dropped)
	rand.Shuffle(len(shuffle), func(i, j int) {
		shuffle[i], shuffle[j] = shuffle[j], shuffle[i]
	})

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), appUUIDs).Return(shuffle, nil)

	result, err := s.service.watchApplicationsWithPendingCharmsMapper(c.Context(), changes)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, expected)
}

func (s *applicationWatcherServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)
	s.storageService = NewMockStorageService(ctrl)

	modelUUID := tc.Must(c, model.NewUUID)

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewWatchableService(
		s.state,
		s.storageService,
		domaintesting.NoopLeaderEnsurer(),
		s.watcherFactory,
		nil,
		nil,
		nil,
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		modelUUID,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)

	c.Cleanup(func() {
		s.state = nil
		s.storageService = nil
		s.charm = nil
		s.watcherFactory = nil
	})

	return ctrl
}

func (s *applicationServiceSuite) TestSetApplicationCharmWithChannel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		ID:     "charm-id",
		Channel: &internalcharm.Channel{
			Risk: internalcharm.Stable,
		},
	}
	params := application.SetCharmParams{
		CharmOrigin: origin,
	}
	channel, err := encodeChannel(params.CharmOrigin.Channel)
	c.Assert(err, tc.ErrorIsNil)
	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return([]application.StorageDirective{}, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(map[string]applicationcharm.Storage{}, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(nil), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil, nil)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).
		Do(func(_ context.Context, _ coreapplication.UUID, _ corecharm.ID, params application.SetCharmStateParams) error {
			c.Assert(params.Channel, tc.DeepEquals, channel)
			return nil
		})

	err = s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestSetApplicationCharmEmptyChannel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		ID:     "charm-id",
	}
	params := application.SetCharmParams{
		CharmOrigin: origin,
	}
	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return([]application.StorageDirective{}, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(map[string]applicationcharm.Storage{}, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(nil), nil)
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).Return(nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil, nil)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetApplicationCharmWithStorageNameAdded tests that adding a new
// storage name in the new charm is allowed.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageNameAdded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			PoolUUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
			Count:            1,
			Size:             1024,
		},
	}

	// Current charm has only "data" storage.
	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
		},
	}

	// New charm adds "logs" storage - this should be allowed.
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
		},
		"logs": {
			Type:        applicationcharm.StorageFilesystem,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 512,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		[]internal.CreateApplicationStorageDirectiveArg{{
			Name:     "logs",
			PoolUUID: tc.Must(c, domainstorage.NewStoragePoolUUID),
			Count:    1,
			Size:     512,
		}},
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: existingStorageDirectives[0].PoolUUID,
			Count:    1,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).Return(nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetApplicationCharmWithStorageNameRemoved tests that removing a storage
// name from the new charm is rejected.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageNameRemoved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	// Current charm has "data" storage.
	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
		},
		"logs": {
			Type:        applicationcharm.StorageFilesystem,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 512,
		},
	}

	// New charm removes "logs" storage - this should be rejected.
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	var storageNameRemovedErr applicationerrors.CharmStorageDefinitionRemoved
	c.Assert(errors.As(err, &storageNameRemovedErr), tc.IsTrue)
	c.Assert(storageNameRemovedErr.StorageName, tc.Equals, "logs")
}

// TestSetApplicationCharmWithStorageTypeChanged tests that changing a storage
// type from the old charm to the new charm is rejected.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageTypeChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	// Current charm has "data" as block storage.
	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
		},
	}

	// New charm changes "data" to filesystem storage - this should be rejected.
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageFilesystem,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	var storageTypeChangedErr applicationerrors.CharmStorageTypeChanged
	c.Assert(errors.As(err, &storageTypeChangedErr), tc.IsTrue)
	c.Assert(storageTypeChangedErr.StorageName, tc.Equals, "data")
	c.Assert(storageTypeChangedErr.OldType, tc.Equals, "block")
	c.Assert(storageTypeChangedErr.NewType, tc.Equals, "filesystem")
}

// TestSetApplicationCharmWithStorageSizeMinimumIncreased tests that increasing
// the minimum size requirement in the new charm is rejected.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageSizeMinimumIncreased(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	// Current charm requires minimum 1024MB.
	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
		},
	}

	// New charm requires minimum 2048MB - this should be rejected.
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 2048,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	var sizeMinViolationErr applicationerrors.CharmStorageDefinitionMinSizeViolation
	c.Assert(errors.As(err, &sizeMinViolationErr), tc.IsTrue)
	c.Assert(sizeMinViolationErr.StorageName, tc.Equals, "data")
	c.Assert(sizeMinViolationErr.ExistingMin, tc.Equals, uint64(1024))
	c.Assert(sizeMinViolationErr.NewMin, tc.Equals, uint64(2048))
}

// TestSetApplicationCharmStorageSizeMinimumDecreased tests that decreasing
// the minimum size requirement in the new charm is allowed.
func (s *applicationServiceSuite) TestSetApplicationCharmStorageSizeMinimumDecreased(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			PoolUUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
			Count:            1,
			Size:             8192,
		},
	}

	// Current charm requires minimum 2048MB.
	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 4096,
		},
	}

	// New charm requires only 1024MB - this should be allowed.
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: existingStorageDirectives[0].PoolUUID,
			Count:    1,
			Size:     8192,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).Return(nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetApplicationCharmWithStorageCountMinimumIncreased tests that increasing
// the minimum count requirement in the new charm is rejected.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageCountMinimumIncreased(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	// Current charm requires minimum count of 1.
	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    5,
			MinimumSize: 1024,
		},
	}

	// New charm requires minimum count of 3 - this should be rejected.
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    3,
			CountMax:    5,
			MinimumSize: 1024,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	var countMinViolationErr applicationerrors.CharmStorageDefinitionMinCountViolation
	c.Assert(errors.As(err, &countMinViolationErr), tc.IsTrue)
	c.Assert(countMinViolationErr.StorageName, tc.Equals, "data")
	c.Assert(countMinViolationErr.ExistingMin, tc.Equals, 1)
	c.Assert(countMinViolationErr.NewMin, tc.Equals, 3)
}

// TestSetApplicationCharmWithStorageCountMinimumDecreased tests that decreasing
// the minimum count requirement in the new charm is allowed.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageCountMinimumDecreased(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			PoolUUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
			Count:            3,
			Size:             1024,
		},
	}

	// Current charm requires minimum count of 3.
	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    3,
			CountMax:    5,
			MinimumSize: 1024,
		},
	}

	// New charm requires minimum count of 1 - this should be allowed.
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    5,
			MinimumSize: 1024,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: existingStorageDirectives[0].PoolUUID,
			Count:    5,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).Return(nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetApplicationCharmWithStorageCountMaximumIncreasedBounded tests that
// increasing the maximum count from a bounded value to a higher bounded value
// is allowed.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageCountMaximumIncreasedBounded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			PoolUUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
			Count:            3,
			Size:             1024,
		},
	}

	// Current charm allows maximum count of 5.
	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    5,
			MinimumSize: 1024,
		},
	}

	// New charm allows maximum count of 10 - this should be allowed.
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    10,
			MinimumSize: 1024,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: existingStorageDirectives[0].PoolUUID,
			Count:    5,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).Return(nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetApplicationCharmWithStorageCountMaximumDecreasedBounded tests that
// decreasing the maximum count from a bounded value to a lower bounded value
// is rejected.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageCountMaximumDecreasedBounded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	// Current charm allows maximum count of 5.
	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    5,
			MinimumSize: 1024,
		},
	}

	// New charm allows maximum count of 3 - this should be rejected.
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    3,
			MinimumSize: 1024,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	var countMaxViolationErr applicationerrors.CharmStorageDefinitionMaxCountViolation
	c.Assert(errors.As(err, &countMaxViolationErr), tc.IsTrue)
	c.Assert(countMaxViolationErr.StorageName, tc.Equals, "data")
	c.Assert(countMaxViolationErr.ExistingMax, tc.Equals, 5)
	c.Assert(countMaxViolationErr.NewMax, tc.Equals, 3)
}

// TestSetApplicationCharmWithStorageCountMaximumUnboundedToBounded tests that
// changing from unbounded (-1) to bounded maximum count is rejected.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageCountMaximumUnboundedToBounded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	// Current charm has unbounded maximum count (-1).
	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    -1,
			MinimumSize: 1024,
		},
	}

	// New charm has bounded maximum count - this should be rejected.
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    100,
			MinimumSize: 1024,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	c.Assert(err, tc.ErrorMatches, `validating new charm storage against existing charm storage: storage definition "data" new maximum count 100 is less than existing maximum count \(unbounded\)`)
	var countMaxViolationErr applicationerrors.CharmStorageDefinitionMaxCountViolation
	c.Assert(errors.As(err, &countMaxViolationErr), tc.IsTrue)
	c.Assert(countMaxViolationErr.StorageName, tc.Equals, "data")
	c.Assert(countMaxViolationErr.ExistingMax, tc.Equals, -1)
	c.Assert(countMaxViolationErr.NewMax, tc.Equals, 100)
}

// TestSetApplicationCharmWithStorageCountMaximumBoundedToUnbounded tests that
// changing from bounded to unbounded (-1) maximum count is allowed.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageCountMaximumBoundedToUnbounded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			PoolUUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
			Count:            5,
			Size:             1024,
		},
	}

	// Current charm has bounded maximum count of 10.
	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    10,
			MinimumSize: 1024,
		},
	}

	// New charm has unbounded maximum count - this should be allowed.
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    -1,
			MinimumSize: 1024,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: existingStorageDirectives[0].PoolUUID,
			Count:    5,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).Return(nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetApplicationCharmWithStorageCountMaximumBothUnbounded tests that
// both charms having unbounded maximum count is allowed.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageCountMaximumBothUnbounded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			PoolUUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
			Count:            5,
			Size:             1024,
		},
	}

	// Both current and new charm have unbounded maximum count.
	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    -1,
			MinimumSize: 1024,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    -1,
			MinimumSize: 1024,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: existingStorageDirectives[0].PoolUUID,
			Count:    5,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).Return(nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetApplicationCharmWithStorageCountInvalidMinMaxCount tests that if the new charm has invalid storage configuration
// (minimum count greater than maximum count), the SetApplicationCharm call is rejected.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageCountInvalidMinMaxCount(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	// New charm storage count min is higher than max, this is invalid.
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    5,
			CountMax:    3,
			MinimumSize: 1024,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	c.Assert(err, tc.ErrorMatches, `.*minimum count 5 greater than maximum count 3.*`)
}

// TestSetApplicationCharmWithStorageCountMaximumSingletonToMultipleWithLocation
// tests that a storage with a fixed location cannot change from singleton to
// multiple.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageCountMaximumSingletonToMultipleWithLocation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageFilesystem,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
			Location:    "/var/lib/data",
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageFilesystem,
			CountMin:    1,
			CountMax:    3,
			MinimumSize: 1024,
			Location:    "/var/lib/data",
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	var singleToMultipleErr applicationerrors.CharmStorageDefinitionSingleToMultipleViolation
	c.Assert(errors.As(err, &singleToMultipleErr), tc.IsTrue)
	c.Assert(singleToMultipleErr.StorageName, tc.Equals, "data")
	c.Assert(singleToMultipleErr.ExistingMax, tc.Equals, 1)
	c.Assert(singleToMultipleErr.NewMax, tc.Equals, 3)
}

// TestSetApplicationCharmWithStorageSingletonWithLocationRangeEqualsSingleton
// tests that a singleton storage with a fixed location remains allowed when
// the new charm keeps the same effective singleton definition.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageSingletonWithLocationRangeEqualsSingleton(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageFilesystem,
			Name:             "data",
			PoolUUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
			Count:            1,
			Size:             1024,
		},
	}

	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageFilesystem,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
			Location:    "/var/lib/data",
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageFilesystem,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
			Location:    "/var/lib/data",
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: existingStorageDirectives[0].PoolUUID,
			Count:    1,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).Return(nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetApplicationCharmWithStorageSharedChanged tests that if the new charm has a storage with a different shared setting
// than the current charm, the SetApplicationCharm call is rejected.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageSharedChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {Type: applicationcharm.StorageFilesystem, CountMin: 0, CountMax: 1, MinimumSize: 1024, Shared: false},
	}
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {Type: applicationcharm.StorageFilesystem, CountMin: 0, CountMax: 1, MinimumSize: 1024, Shared: true},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	var got applicationerrors.CharmStorageDefinitionSharedChanged
	c.Assert(errors.As(err, &got), tc.IsTrue)
	c.Assert(got.StorageName, tc.Equals, "data")
	c.Assert(got.ExistingValue, tc.Equals, false)
	c.Assert(got.NewValue, tc.Equals, true)
}

// TestSetApplicationCharmWithStorageReadOnlyChanged tests that if the new charm has a storage with a different read-only setting
// than the current charm, the SetApplicationCharm call is rejected.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageReadOnlyChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {Type: applicationcharm.StorageFilesystem, CountMin: 1, CountMax: 1, MinimumSize: 1024, ReadOnly: false},
	}
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {Type: applicationcharm.StorageFilesystem, CountMin: 1, CountMax: 1, MinimumSize: 1024, ReadOnly: true},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	var got applicationerrors.CharmStorageDefinitionReadOnlyChanged
	c.Assert(errors.As(err, &got), tc.IsTrue)
	c.Assert(got.StorageName, tc.Equals, "data")
	c.Assert(got.ExistingValue, tc.Equals, false)
	c.Assert(got.NewValue, tc.Equals, true)
}

// TestSetApplicationCharmWithStorageLocationChanged tests that if the new charm has a storage with a different location
// than the current charm, the SetApplicationCharm call is rejected.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageLocationChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)

	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {Type: applicationcharm.StorageFilesystem, CountMin: 1, CountMax: 1, MinimumSize: 1024, Location: "/a"},
	}
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {Type: applicationcharm.StorageFilesystem, CountMin: 1, CountMax: 1, MinimumSize: 1024, Location: "/b"},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	var got applicationerrors.CharmStorageDefinitionLocationChanged
	c.Assert(errors.As(err, &got), tc.IsTrue)
	c.Assert(got.StorageName, tc.Equals, "data")
	c.Assert(got.ExistingLocation, tc.Equals, "/a")
	c.Assert(got.NewLocation, tc.Equals, "/b")
}

// TestSetApplicationCharmWithStorageDirectivesChanges tests that when storage directives
// are changed in the new charm, they are correctly reconciled and passed to the state.
// While existingStorageDirectives does not adhere to current charm storage requirements,
// we want to test that even if the existing directives have been manually updated to a "bad" state,
// the new directives will still be correctly calculated and passed to the state.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)

	// Combination of: update storage count, update storage size, and add new storage.
	// Note that existing storage directives do not adhere to current charm storage requirements.
	existingStorageDirectives := []application.StorageDirective{
		// storage "data" size and count will be updated.
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,   // Will be increased to 2
			Size:             512, // Will be updated to 1024
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	currentCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    2,
			CountMax:    5,
			MinimumSize: 1024,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    2,
			CountMax:    5,
			MinimumSize: 1024,
		},
		// storage "logs" will be created.
		"logs": {
			Type:        applicationcharm.StorageFilesystem,
			CountMin:    1,
			CountMax:    3,
			MinimumSize: 512,
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(currentCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		[]internal.CreateApplicationStorageDirectiveArg{{
			Name:     "logs",
			PoolUUID: *modelStoragePools.FilesystemPoolUUID,
			Count:    1,
			Size:     512,
		}},
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: *modelStoragePools.BlockDevicePoolUUID,
			Count:    2,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	// Expect complex changes: update data count, delete cache, add logs
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).
		Do(func(_ context.Context, _ coreapplication.UUID, _ corecharm.ID, params application.SetCharmStateParams) error {
			c.Assert(params.StorageDirectivesToCreate, tc.HasLen, 1)
			c.Assert(params.StorageDirectivesToUpdate, tc.HasLen, 1)

			c.Assert(params.StorageDirectivesToCreate[0].Name.String(), tc.Equals, "logs")
			c.Assert(params.StorageDirectivesToCreate[0].PoolUUID, tc.Equals, *modelStoragePools.FilesystemPoolUUID)
			c.Assert(params.StorageDirectivesToCreate[0].Count, tc.Equals, uint32(1))
			c.Assert(params.StorageDirectivesToCreate[0].Size, tc.Equals, uint64(512))

			c.Assert(params.StorageDirectivesToUpdate[0].Name.String(), tc.Equals, "data")
			c.Assert(params.StorageDirectivesToUpdate[0].PoolUUID, tc.Equals, *modelStoragePools.BlockDevicePoolUUID)
			c.Assert(params.StorageDirectivesToUpdate[0].Count, tc.Equals, uint32(2))
			c.Assert(params.StorageDirectivesToUpdate[0].Size, tc.Equals, uint64(1024))

			return nil
		})

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, application.SetCharmParams{})
	c.Assert(err, tc.IsNil)
}

// TestSetApplicationCharmWithStorageDirectivesOverrideCountOverflow tests that
// when a storage directive override sets a very large count, an error is returned.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverrideCountOverflow(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)
	count := uint32(math.MaxUint32)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    3,
			MinimumSize: 512,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"data": {
				Count: ptr(count),
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: *modelStoragePools.BlockDevicePoolUUID,
			Count:    1,
			Size:     512,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		applicationerrors.StorageCountLimitExceeded{
			Maximum:     ptr(3),
			Minimum:     1,
			Requested:   int(count),
			StorageName: "data",
		},
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.ErrorMatches, `validating storage directives: storage "data" cannot exceed 3 storage instances`)
}

// TestSetApplicationCharmWithStorageDirectivesOverridePoolChange tests that
// when a storage directive override changes the pool, the change is applied
// correctly.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverridePoolChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    2,
			CountMax:    5,
			MinimumSize: 512,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"data": {
				PoolUUID: &poolUUID,
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: *modelStoragePools.BlockDevicePoolUUID,
			Count:    2,
			Size:     512,
		}},
		nil,
	)
	// Expect the override pool UUID validation to succeed.
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ map[string]internal.ValidateStorageArg, overrides map[string]storage.StorageDirectiveOverride) error {
			override, exists := overrides["data"]
			c.Assert(exists, tc.IsTrue)
			c.Assert(override.PoolUUID, tc.NotNil)
			c.Assert(*override.PoolUUID, tc.Equals, poolUUID)
			return nil
		},
	)
	// Expect the pool change to be applied in the update directives.
	// Count should still be updated to the new charm minimum, but size should remain unchanged as there is no override for it.
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).Do(
		func(_ context.Context, _ coreapplication.UUID, _ corecharm.ID, params application.SetCharmStateParams) error {
			c.Assert(params.StorageDirectivesToCreate, tc.HasLen, 0)
			c.Assert(params.StorageDirectivesToUpdate, tc.HasLen, 1)

			c.Assert(params.StorageDirectivesToUpdate[0].Name.String(), tc.Equals, "data")
			c.Assert(params.StorageDirectivesToUpdate[0].PoolUUID, tc.Equals, poolUUID)
			c.Assert(params.StorageDirectivesToUpdate[0].Count, tc.Equals, uint32(2))
			return nil
		},
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.IsNil)
}

// TestSetApplicationCharmWithStorageDirectivesOverridePoolChangeUnsupported tests that
// when a storage directive override changes the pool to one that does not support
// the new charm storage type (i.e pool does not support filesystem), an error is returned.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverridePoolChangeUnsupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    2,
			CountMax:    5,
			MinimumSize: 512,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"data": {
				PoolUUID: &poolUUID,
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: *modelStoragePools.BlockDevicePoolUUID,
			Count:    2,
			Size:     512,
		}},
		nil,
	)
	// Expect the override pool UUID validation to fail.
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ map[string]internal.ValidateStorageArg, overrides map[string]storage.StorageDirectiveOverride) error {
			override, exists := overrides["data"]
			c.Assert(exists, tc.IsTrue)
			c.Assert(override.PoolUUID, tc.NotNil)
			c.Assert(*override.PoolUUID, tc.Equals, poolUUID)
			return errors.Errorf("storage directive pool %q does not support charm storage type filesystem", poolUUID.String())
		},
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.ErrorMatches, "validating storage directives: storage directive pool .* does not support charm storage type filesystem")
}

// TestSetApplicationCharmWithStorageDirectivesOverridePoolChangeUnknownPool tests that
// when a storage directive override changes the pool to one that does not exist in the model,
// an error is returned.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverridePoolChangeUnknownPool(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)
	unknownPoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    3,
			MinimumSize: 512,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"data": {
				PoolUUID: &unknownPoolUUID,
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: *modelStoragePools.BlockDevicePoolUUID,
			Count:    1,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		errors.Errorf(`storage directive data references unknown pool %q`, unknownPoolUUID),
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.ErrorMatches, `validating storage directives: storage directive data references unknown pool ".*"`)
}

// TestSetApplicationCharmWithStorageDirectivesOverrideCountWithinRange tests that
// when a storage directive override changes the count, the count is applied
// correctly within the range of the new minimum and maximum.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverrideCountWithinRange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    2,
			CountMax:    5,
			MinimumSize: 512,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"data": {
				Count: ptr(uint32(4)),
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: *modelStoragePools.BlockDevicePoolUUID,
			Count:    1,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ map[string]internal.ValidateStorageArg, overrides map[string]storage.StorageDirectiveOverride) error {
			override, exists := overrides["data"]
			c.Assert(exists, tc.IsTrue)
			c.Assert(override.Count, tc.NotNil)
			c.Assert(*override.Count, tc.Equals, uint32(4))
			return nil
		},
	)
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).Do(
		func(_ context.Context, _ coreapplication.UUID, _ corecharm.ID, params application.SetCharmStateParams) error {
			c.Assert(params.StorageDirectivesToUpdate, tc.HasLen, 1)
			c.Assert(params.StorageDirectivesToUpdate[0].Name.String(), tc.Equals, "data")
			c.Assert(params.StorageDirectivesToUpdate[0].PoolUUID, tc.Equals, *modelStoragePools.BlockDevicePoolUUID)
			c.Assert(params.StorageDirectivesToUpdate[0].Count, tc.Equals, uint32(4))
			return nil
		},
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.IsNil)
}

// TestSetApplicationCharmWithStorageDirectivesOverrideCountBelowMinimum tests that
// when a storage directive override changes the count below the new charm minimum count,
// an error is returned.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverrideCountBelowMinimum(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	countMax := 8
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    3,
			CountMax:    countMax,
			MinimumSize: 512,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"data": {
				Count: ptr(uint32(2)),
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: *modelStoragePools.BlockDevicePoolUUID,
			Count:    1,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ map[string]internal.ValidateStorageArg, overrides map[string]storage.StorageDirectiveOverride) error {
			override, exists := overrides["data"]
			c.Assert(exists, tc.IsTrue)
			c.Assert(override.Count, tc.NotNil)
			c.Assert(*override.Count, tc.Equals, uint32(2))

			return applicationerrors.StorageCountLimitExceeded{
				Maximum:     &countMax,
				Minimum:     3,
				Requested:   2,
				StorageName: "data",
			}
		},
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.ErrorMatches, `validating storage directives: storage "data" cannot have less than 3 storage instances`)
}

// TestSetApplicationCharmWithStorageDirectivesOverrideCountAboveMaximum tests that
// when a storage directive override changes the count above the new charm maximum count,
// an error is returned.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverrideCountAboveMaximum(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	countMax := 5
	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    2,
			CountMax:    countMax,
			MinimumSize: 512,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"data": {
				Count: ptr(uint32(6)),
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: *modelStoragePools.BlockDevicePoolUUID,
			Count:    1,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ map[string]internal.ValidateStorageArg, overrides map[string]storage.StorageDirectiveOverride) error {
			override, exists := overrides["data"]
			c.Assert(exists, tc.IsTrue)
			c.Assert(override.Count, tc.NotNil)
			c.Assert(*override.Count, tc.Equals, uint32(6))
			return applicationerrors.StorageCountLimitExceeded{
				Maximum:     &countMax,
				Minimum:     2,
				Requested:   6,
				StorageName: "data",
			}
		},
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.ErrorMatches, `validating storage directives: storage "data" cannot exceed 5 storage instances`)
}

// TestSetApplicationCharmWithStorageDirectivesOverrideSizeAboveMinimum tests that
// when a storage directive override changes the size above the new charm minimum size,
// the size is applied correctly.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverrideSizeAboveMinimum(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    3,
			MinimumSize: 1024,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"data": {
				Size:     ptr(uint64(2048)),
				PoolUUID: modelStoragePools.BlockDevicePoolUUID,
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: *modelStoragePools.BlockDevicePoolUUID,
			Count:    1,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ map[string]internal.ValidateStorageArg, overrides map[string]storage.StorageDirectiveOverride) error {
			override, exists := overrides["data"]
			c.Assert(exists, tc.IsTrue)
			c.Assert(override.Size, tc.NotNil)
			c.Assert(*override.Size, tc.Equals, uint64(2048))
			return nil
		},
	)
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).Do(
		func(_ context.Context, _ coreapplication.UUID, _ corecharm.ID, params application.SetCharmStateParams) error {
			c.Assert(params.StorageDirectivesToUpdate, tc.HasLen, 1)
			c.Assert(params.StorageDirectivesToUpdate[0].Name.String(), tc.Equals, "data")
			c.Assert(params.StorageDirectivesToUpdate[0].Size, tc.Equals, uint64(2048))
			return nil
		},
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.IsNil)
}

// TestSetApplicationCharmWithStorageDirectivesOverrideSizeBelowMinimum tests that
// when a storage directive override changes the size below the new charm minimum size,
// an error is returned.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverrideSizeBelowMinimum(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    3,
			MinimumSize: 1024,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"data": {
				Size:     ptr(uint64(512)),
				PoolUUID: modelStoragePools.BlockDevicePoolUUID,
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: *modelStoragePools.BlockDevicePoolUUID,
			Count:    1,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ map[string]internal.ValidateStorageArg, overrides map[string]storage.StorageDirectiveOverride) error {
			override, exists := overrides["data"]
			c.Assert(exists, tc.IsTrue)
			c.Assert(override.Size, tc.NotNil)
			c.Assert(*override.Size, tc.Equals, uint64(512))
			return errors.Errorf(
				"storage directive size %d is less than the charm minimum requirement of %d",
				*override.Size,
				uint64(1024),
			)
		},
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.ErrorMatches, `validating storage directives: storage directive size 512 is less than the charm minimum requirement of 1024`)
}

// TestSetApplicationCharmWithStorageDirectivesOverrideSizeEqualsMinimum tests that
// when a storage directive override changes the size to the new charm minimum size,
// the size is applied correctly.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverrideSizeEqualsMinimum(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    3,
			MinimumSize: 1024,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"data": {
				Size:     ptr(uint64(1024)),
				PoolUUID: modelStoragePools.BlockDevicePoolUUID,
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil,
		[]internal.UpdateApplicationStorageDirectiveArg{{
			Name:     "data",
			PoolUUID: *modelStoragePools.BlockDevicePoolUUID,
			Count:    1,
			Size:     1024,
		}},
		nil,
	)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ map[string]internal.ValidateStorageArg, overrides map[string]storage.StorageDirectiveOverride) error {
			override, exists := overrides["data"]
			c.Assert(exists, tc.IsTrue)
			c.Assert(override.Size, tc.NotNil)
			c.Assert(*override.Size, tc.Equals, uint64(1024))
			return nil
		},
	)
	s.state.EXPECT().SetApplicationCharm(gomock.Any(), appUUID, charmID, gomock.Any()).Do(
		func(_ context.Context, _ coreapplication.UUID, _ corecharm.ID, params application.SetCharmStateParams) error {
			c.Assert(params.StorageDirectivesToUpdate, tc.HasLen, 1)
			c.Assert(params.StorageDirectivesToUpdate[0].Name.String(), tc.Equals, "data")
			c.Assert(params.StorageDirectivesToUpdate[0].Size, tc.Equals, uint64(1024))
			return nil
		},
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.IsNil)
}

// TestSetApplicationCharmWithStorageDirectivesOverrideNonExistentDirective tests that
// when a storage directive override does not exist in the new charm, an error is returned.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverrideNonExistentDirective(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    5,
			MinimumSize: 512,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"not-data": {
				Size:     ptr(uint64(1024)),
				PoolUUID: modelStoragePools.BlockDevicePoolUUID,
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil, nil)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ map[string]internal.ValidateStorageArg, overrides map[string]storage.StorageDirectiveOverride) error {
			_, exists := overrides["data"]
			c.Assert(exists, tc.IsFalse)
			return errors.New("storage directive not-data does not exist in the application")
		},
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.ErrorMatches, `validating storage directives: storage directive not-data does not exist in the application`)
}

// TestSetApplicationCharmWithStorageDirectivesOverrideInvalidPool tests that
// when a storage directive override specifies an invalid pool, an error is returned.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverrideInvalidPool(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)
	invalidPoolUUID := domainstorage.StoragePoolUUID("")

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    5,
			MinimumSize: 512,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"not-data": {
				Size:     ptr(uint64(1024)),
				PoolUUID: &invalidPoolUUID,
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil, nil)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		errors.New(`storage directive "not-data" references an invalid pool uuid ""`),
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.ErrorMatches, `validating storage directives: storage directive "not-data" references an invalid pool uuid ""`)
}

// TestSetApplicationCharmWithStorageDirectivesOverrideMissingPool tests that
// when a storage directive override specifies a pool that is not found in the model, an error is returned.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverrideMissingPool(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)
	missingPoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    5,
			MinimumSize: 512,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"data": {
				Size:     ptr(uint64(2048)),
				PoolUUID: &missingPoolUUID,
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil, nil)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		errors.Errorf(`storage directive data references unknown pool %q`, missingPoolUUID),
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.ErrorMatches, `validating storage directives: storage directive data references unknown pool ".*"`)
}

// TestSetApplicationCharmWithStorageDirectivesOverrideValidAndInvalid tests that
// when both a valid and invalid storage directive is supplied, an error will still be returned.
func (s *applicationServiceSuite) TestSetApplicationCharmWithStorageDirectivesOverrideValidAndInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmID := charmtesting.GenCharmID(c)
	modelStoragePools := makeModelStoragePools(c)

	existingStorageDirectives := []application.StorageDirective{
		{
			CharmStorageType: applicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         *modelStoragePools.BlockDevicePoolUUID,
		},
	}

	newCharmStorages := map[string]applicationcharm.Storage{
		"data": {
			Type:        applicationcharm.StorageBlock,
			CountMin:    1,
			CountMax:    5,
			MinimumSize: 512,
		},
	}

	params := application.SetCharmParams{
		StorageDirectiveOverrides: map[string]application.ApplicationStorageDirectiveOverride{
			"data": {
				Size:     ptr(uint64(2048)),
				PoolUUID: modelStoragePools.BlockDevicePoolUUID,
			},
			"not-data": {
				Size:     ptr(uint64(1024)),
				PoolUUID: modelStoragePools.BlockDevicePoolUUID,
			},
		},
	}

	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.state.EXPECT().GetCharmID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(charmID, nil)
	s.storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(existingStorageDirectives, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), charmID).Return(newCharmStorages, nil)
	s.state.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appUUID).Return(makeCharmWithStorage(newCharmStorages), nil)
	s.storageService.EXPECT().ReconcileStorageDirectivesAgainstCharmStorage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil, nil)
	s.storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ map[string]internal.ValidateStorageArg, overrides map[string]storage.StorageDirectiveOverride) error {
			c.Assert(len(overrides), tc.Equals, 2)
			c.Assert(overrides["data"].Size, tc.NotNil)
			c.Assert(*overrides["data"].Size, tc.Equals, uint64(2048))
			c.Assert(overrides["not-data"].Size, tc.NotNil)
			c.Assert(*overrides["not-data"].Size, tc.Equals, uint64(1024))

			return errors.New("storage directive not-data does not exist in the application")
		},
	)

	err := s.service.SetApplicationCharm(c.Context(), appName, applicationcharm.CharmLocator{}, params)
	c.Assert(err, tc.ErrorMatches, `validating storage directives: storage directive not-data does not exist in the application`)
}

func (s *applicationServiceSuite) TestGetApplicationLife(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationLife(gomock.Any(), appUUID).Return(life.Alive, nil)

	appLife, err := s.service.GetApplicationLife(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appLife, tc.Equals, corelife.Alive)
}

func (s *applicationServiceSuite) TestGetApplicationLifeInvalidAppUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationLife(c.Context(), "!!!")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func (s *applicationServiceSuite) TestGetApplicationDetails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetApplicationDetails(gomock.Any(), appUUID).Return(application.ApplicationDetails{
		Life: life.Alive,
		Name: "foo",
	}, nil)

	appDetails, err := s.service.GetApplicationDetails(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appDetails, tc.DeepEquals, application.ApplicationDetails{
		Life: life.Alive,
		Name: "foo",
	})
}

func (s *applicationServiceSuite) TestGetApplicationDetailsInvalidAppUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationDetails(c.Context(), "!!!")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func makeModelStoragePools(c *tc.C) internal.ModelStoragePools {
	fakeFilesytemPoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	fakeBlockdevicePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	return internal.ModelStoragePools{
		FilesystemPoolUUID:  &fakeFilesytemPoolUUID,
		BlockDevicePoolUUID: &fakeBlockdevicePoolUUID,
	}
}

// makeCharmWithStorage creates a minimal charm with the given storage map for testing
func makeCharmWithStorage(storage map[string]applicationcharm.Storage) applicationcharm.Charm {
	return applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Storage: storage,
			RunAs:   "default",
		},
	}
}
