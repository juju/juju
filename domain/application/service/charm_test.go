// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/state/watcher/watchertest"
)

type charmServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&charmServiceSuite{})

func (s *charmServiceSuite) TestGetCharmIDWithoutRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(context.Background(), charm.GetCharmArgs{
		Name:   "foo",
		Source: charm.CharmHubSource,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmIDWithoutSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(context.Background(), charm.GetCharmArgs{
		Name:     "foo",
		Revision: ptr(42),
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmSourceNotValid)
}

func (s *charmServiceSuite) TestGetCharmIDInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(context.Background(), charm.GetCharmArgs{
		Name: "Foo",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *charmServiceSuite) TestGetCharmIDInvalidSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(context.Background(), charm.GetCharmArgs{
		Name:     "foo",
		Revision: ptr(42),
		Source:   "wrong-source",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmSourceNotValid)
}

func (s *charmServiceSuite) TestGetCharmID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	rev := 42

	s.state.EXPECT().GetCharmID(gomock.Any(), "foo", rev, charm.LocalSource).Return(id, nil)

	result, err := s.service.GetCharmID(context.Background(), charm.GetCharmArgs{
		Name:     "foo",
		Revision: &rev,
		Source:   charm.LocalSource,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, id)
}

func (s *charmServiceSuite) TestIsControllerCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().IsControllerCharm(gomock.Any(), id).Return(true, nil)

	result, err := s.service.IsControllerCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *charmServiceSuite) TestIsControllerCharmCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().IsControllerCharm(gomock.Any(), id).Return(false, applicationerrors.CharmNotFound)

	_, err := s.service.IsControllerCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestIsControllerCharmInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.IsControllerCharm(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestIsCharmAvailable(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().IsCharmAvailable(gomock.Any(), id).Return(true, nil)

	result, err := s.service.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *charmServiceSuite) TestIsCharmAvailableCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().IsCharmAvailable(gomock.Any(), id).Return(false, applicationerrors.CharmNotFound)

	_, err := s.service.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestIsCharmAvailableInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.IsCharmAvailable(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestSupportsContainers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().SupportsContainers(gomock.Any(), id).Return(true, nil)

	result, err := s.service.SupportsContainers(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *charmServiceSuite) TestSupportsContainersCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().SupportsContainers(gomock.Any(), id).Return(false, applicationerrors.CharmNotFound)

	_, err := s.service.SupportsContainers(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestSupportsContainersInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.SupportsContainers(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestGetCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Conversion of the metadata tests is done in the types package.

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(charm.Charm{
		Metadata: charm.Metadata{
			Name: "foo",

			// RunAs becomes mandatory when being persisted. Empty string is not
			// allowed.
			RunAs: "default",
		},
		Source:   charm.LocalSource,
		Revision: 42,
	}, nil, nil)

	metadata, locator, isAvailable, err := s.service.GetCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata.Meta(), gc.DeepEquals, &internalcharm.Meta{
		Name: "foo",

		// Notice that the RunAs field becomes empty string when being returned.
	})
	c.Check(locator, gc.Equals, charm.CharmLocator{
		Source:   charm.LocalSource,
		Revision: 42,
	})
	c.Check(isAvailable, gc.Equals, true)
}

func (s *charmServiceSuite) TestGetCharmCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(charm.Charm{}, nil, applicationerrors.CharmNotFound)

	_, _, _, err := s.service.GetCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, _, err := s.service.GetCharm(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestGetCharmMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Conversion of the metadata tests is done in the types package.

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmMetadata(gomock.Any(), id).Return(charm.Metadata{
		Name: "foo",

		// RunAs becomes mandatory when being persisted. Empty string is not
		// allowed.
		RunAs: "default",
	}, nil)

	metadata, err := s.service.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata, gc.DeepEquals, internalcharm.Meta{
		Name: "foo",

		// Notice that the RunAs field becomes empty string when being returned.
	})
}

func (s *charmServiceSuite) TestGetCharmMetadataCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmMetadata(gomock.Any(), id).Return(charm.Metadata{}, applicationerrors.CharmNotFound)

	_, err := s.service.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmMetadataInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmMetadata(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestGetCharmLXDProfile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmLXDProfile(gomock.Any(), id).Return([]byte(`{"config": {"foo":"bar"}, "description": "description", "devices": {"gpu":{"baz": "x"}}}`), 42, nil)

	profile, revision, err := s.service.GetCharmLXDProfile(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(profile, gc.DeepEquals, internalcharm.LXDProfile{
		Config: map[string]string{
			"foo": "bar",
		},
		Description: "description",
		Devices: map[string]map[string]string{
			"gpu": {
				"baz": "x",
			},
		},
	})
	c.Check(revision, gc.Equals, 42)
}

func (s *charmServiceSuite) TestGetCharmLXDProfileCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmLXDProfile(gomock.Any(), id).Return(nil, -1, applicationerrors.CharmNotFound)

	_, _, err := s.service.GetCharmLXDProfile(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmLXDProfileInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.GetCharmLXDProfile(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestGetCharmMetadataName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmMetadataName(gomock.Any(), id).Return("name for a charm", nil)

	name, err := s.service.GetCharmMetadataName(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, "name for a charm")
}

func (s *charmServiceSuite) TestGetCharmMetadataNameCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmMetadataName(gomock.Any(), id).Return("", applicationerrors.CharmNotFound)

	_, err := s.service.GetCharmMetadataName(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmMetadataNameInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmMetadataName(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestGetCharmMetadataDescription(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmMetadataDescription(gomock.Any(), id).Return("description for a charm", nil)

	description, err := s.service.GetCharmMetadataDescription(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(description, gc.Equals, "description for a charm")
}

func (s *charmServiceSuite) TestGetCharmMetadataDescriptionCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmMetadataDescription(gomock.Any(), id).Return("", applicationerrors.CharmNotFound)

	_, err := s.service.GetCharmMetadataDescription(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmMetadataDescriptionInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmMetadataDescription(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestGetCharmMetadataStorage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), id).Return(map[string]charm.Storage{
		"foo": {
			Name:        "foo",
			Description: "description",
			Type:        charm.StorageBlock,
			Location:    "/foo",
		},
	}, nil)

	storage, err := s.service.GetCharmMetadataStorage(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(storage, gc.DeepEquals, map[string]internalcharm.Storage{
		"foo": {
			Name:        "foo",
			Description: "description",
			Type:        internalcharm.StorageBlock,
			Location:    "/foo",
		},
	})
}

func (s *charmServiceSuite) TestGetCharmMetadataStorageCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), id).Return(nil, applicationerrors.CharmNotFound)

	_, err := s.service.GetCharmMetadataStorage(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmMetadataResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmMetadataResources(gomock.Any(), id).Return(map[string]charm.Resource{
		"foo": {
			Name:        "foo",
			Type:        charm.ResourceTypeFile,
			Description: "description",
			Path:        "/foo",
		},
	}, nil)

	resources, err := s.service.GetCharmMetadataResources(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resources, gc.DeepEquals, map[string]resource.Meta{
		"foo": {
			Name:        "foo",
			Type:        resource.TypeFile,
			Description: "description",
			Path:        "/foo",
		},
	})
}

func (s *charmServiceSuite) TestGetCharmMetadataResourcesCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmMetadataResources(gomock.Any(), id).Return(nil, applicationerrors.CharmNotFound)

	_, err := s.service.GetCharmMetadataResources(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmArchivePath(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmArchivePath(gomock.Any(), id).Return("archive-path", nil)

	path, err := s.service.GetCharmArchivePath(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "archive-path")
}

func (s *charmServiceSuite) TestGetCharmArchivePathCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmArchivePath(gomock.Any(), id).Return("archive-path", applicationerrors.CharmNotFound)

	_, err := s.service.GetCharmArchivePath(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmArchivePathInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmArchivePath(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestGetCharmArchive(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)
	archive := io.NopCloser(strings.NewReader("archive-content"))

	s.state.EXPECT().GetCharmArchiveMetadata(gomock.Any(), id).Return("archive-path", "hash", nil)
	s.charmStore.EXPECT().Get(gomock.Any(), "archive-path").Return(archive, nil)

	reader, hash, err := s.service.GetCharmArchive(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(hash, gc.Equals, "hash")

	content, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(content), gc.Equals, "archive-content")
}

func (s *charmServiceSuite) TestGetCharmArchiveBySHA256Prefix(c *gc.C) {
	defer s.setupMocks(c).Finish()

	archive := io.NopCloser(strings.NewReader("archive-content"))

	s.charmStore.EXPECT().GetBySHA256Prefix(gomock.Any(), "prefix").Return(archive, nil)

	reader, err := s.service.GetCharmArchiveBySHA256Prefix(context.Background(), "prefix")
	c.Assert(err, jc.ErrorIsNil)

	content, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(content), gc.Equals, "archive-content")
}

func (s *charmServiceSuite) TestGetCharmArchiveCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmArchiveMetadata(gomock.Any(), id).Return("", "", applicationerrors.CharmNotFound)

	_, _, err := s.service.GetCharmArchive(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestSetCharmAvailable(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().SetCharmAvailable(gomock.Any(), id).Return(nil)

	err := s.service.SetCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmServiceSuite) TestSetCharmAvailableCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().SetCharmAvailable(gomock.Any(), id).Return(applicationerrors.CharmNotFound)

	err := s.service.SetCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestSetCharmAvailableInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetCharmAvailable(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestSetCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.charm.EXPECT().Actions().Return(&internalcharm.Actions{})
	s.charm.EXPECT().Config().Return(&internalcharm.Config{})

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	var downloadInfo *charm.DownloadInfo

	s.state.EXPECT().SetCharm(gomock.Any(), charm.Charm{
		Metadata: charm.Metadata{
			Name:  "foo",
			RunAs: "default",
		},
		Manifest:      s.minimalManifest(c),
		ReferenceName: "baz",
		Source:        charm.LocalSource,
		Revision:      1,
		Architecture:  architecture.AMD64,
	}, downloadInfo).Return(id, nil)

	got, warnings, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "baz",
		Revision:      1,
		Architecture:  arch.AMD64,
		DownloadInfo:  downloadInfo,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(warnings, gc.HasLen, 0)
	c.Check(got, gc.DeepEquals, id)
}

func (s *charmServiceSuite) TestSetCharmCharmhubWithNoDownloadInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	var downloadInfo *charm.DownloadInfo

	_, _, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.CharmHub,
		ReferenceName: "baz",
		Revision:      1,
		Architecture:  arch.AMD64,
		DownloadInfo:  downloadInfo,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmDownloadInfoNotFound)
}

func (s *charmServiceSuite) TestSetCharmCharmhub(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.charm.EXPECT().Actions().Return(&internalcharm.Actions{})
	s.charm.EXPECT().Config().Return(&internalcharm.Config{})

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Stable},
		Architectures: []string{"amd64"},
	}}}).MinTimes(1)

	downloadInfo := &charm.DownloadInfo{
		Provenance:         charm.ProvenanceDownload,
		CharmhubIdentifier: "foo",
		DownloadURL:        "http://example.com/foo",
		DownloadSize:       42,
	}

	s.state.EXPECT().SetCharm(gomock.Any(), charm.Charm{
		Metadata: charm.Metadata{
			Name:  "foo",
			RunAs: "default",
		},
		Manifest:      s.minimalManifest(c),
		ReferenceName: "baz",
		Source:        charm.CharmHubSource,
		Revision:      1,
		Architecture:  architecture.AMD64,
	}, downloadInfo).Return(id, nil)

	got, warnings, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.CharmHub,
		ReferenceName: "baz",
		Revision:      1,
		Architecture:  arch.AMD64,
		DownloadInfo:  downloadInfo,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(warnings, gc.HasLen, 0)
	c.Check(got, gc.DeepEquals, id)
}

func (s *charmServiceSuite) TestSetCharmNoName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{})

	_, _, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:        s.charm,
		Source:       corecharm.Local,
		Revision:     1,
		Architecture: arch.AMD64,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *charmServiceSuite) TestSetCharmInvalidSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
	})
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	_, _, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        "charmstore",
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmSourceNotValid)
}

func (s *charmServiceSuite) TestSetCharmRelationNameConflict(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
		Provides: map[string]internalcharm.Relation{
			"foo": {
				Name:  "foo",
				Role:  internalcharm.RoleProvider,
				Scope: internalcharm.ScopeGlobal,
			},
		},
		Requires: map[string]internalcharm.Relation{
			"foo": {
				Name:  "foo",
				Role:  internalcharm.RoleRequirer,
				Scope: internalcharm.ScopeGlobal,
			},
		},
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	_, _, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmRelationNameConflict)
}

func (s *charmServiceSuite) TestSetCharmRelationUnknownRole(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
		Provides: map[string]internalcharm.Relation{
			"foo": {
				Name:  "foo",
				Role:  internalcharm.RelationRole("unknown"),
				Scope: internalcharm.ScopeGlobal,
			},
		},
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	_, _, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmRelationRoleNotValid)
}

func (s *charmServiceSuite) TestSetCharmRelationRoleMismatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
		Provides: map[string]internalcharm.Relation{
			"foo": {
				Name:  "foo",
				Role:  internalcharm.RolePeer,
				Scope: internalcharm.ScopeGlobal,
			},
		},
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	_, _, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmRelationRoleNotValid)
}

func (s *charmServiceSuite) TestSetCharmRelationToReservedNameJuju(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
		Provides: map[string]internalcharm.Relation{
			"foo": {
				Name:      "foo",
				Role:      internalcharm.RoleProvider,
				Scope:     internalcharm.ScopeGlobal,
				Interface: "juju",
			},
		},
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	_, _, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmRelationReservedNameMisuse)
}

func (s *charmServiceSuite) TestSetCharmRelationToReservedNameJujuBlah(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
		Peers: map[string]internalcharm.Relation{
			"foo": {
				Name:      "foo",
				Role:      internalcharm.RolePeer,
				Scope:     internalcharm.ScopeGlobal,
				Interface: "juju-blah",
			},
		},
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	_, _, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmRelationReservedNameMisuse)
}

func (s *charmServiceSuite) TestSetCharmRelationNameToReservedNameJuju(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
		Provides: map[string]internalcharm.Relation{
			"juju": {
				Name:      "juju",
				Role:      internalcharm.RoleProvider,
				Scope:     internalcharm.ScopeGlobal,
				Interface: "foo",
			},
		},
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	_, _, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmRelationReservedNameMisuse)
}

func (s *charmServiceSuite) TestSetCharmRelationNameToReservedNameJujuBlah(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
		Peers: map[string]internalcharm.Relation{
			"juju-blah": {
				Name:      "juju-blah",
				Role:      internalcharm.RolePeer,
				Scope:     internalcharm.ScopeGlobal,
				Interface: "foo",
			},
		},
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	_, _, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmRelationReservedNameMisuse)
}

func (s *charmServiceSuite) TestSetCharmRequireRelationToReservedNameSucceeds(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.charm.EXPECT().Actions().Return(&internalcharm.Actions{})
	s.charm.EXPECT().Config().Return(&internalcharm.Config{})

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
		Requires: map[string]internalcharm.Relation{
			"blah": {
				Name:      "blah",
				Role:      internalcharm.RoleRequirer,
				Scope:     internalcharm.ScopeGlobal,
				Interface: "juju-blah",
			},
		},
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	s.state.EXPECT().SetCharm(gomock.Any(), gomock.Any(), nil).Return(id, nil)

	got, warnings, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(warnings, gc.HasLen, 0)
	c.Check(got, gc.DeepEquals, id)
}

func (s *charmServiceSuite) TestSetCharmRequireRelationNameToReservedName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
		Requires: map[string]internalcharm.Relation{
			"juju-blah": {
				Name:      "juju-blah",
				Role:      internalcharm.RoleRequirer,
				Scope:     internalcharm.ScopeGlobal,
				Interface: "foo",
			},
		},
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	_, _, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmRelationReservedNameMisuse)
}

func (s *charmServiceSuite) TestSetCharmRelationToReservedNameWithSpecialCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.charm.EXPECT().Actions().Return(&internalcharm.Actions{})
	s.charm.EXPECT().Config().Return(&internalcharm.Config{})

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "juju-foo",
		Peers: map[string]internalcharm.Relation{
			"juju-blah": {
				Name:      "juju-blah",
				Role:      internalcharm.RolePeer,
				Scope:     internalcharm.ScopeGlobal,
				Interface: "juju-blah",
			},
		},
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	s.state.EXPECT().SetCharm(gomock.Any(), gomock.Any(), nil).Return(id, nil)

	got, warnings, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(warnings, gc.HasLen, 0)
	c.Check(got, gc.DeepEquals, id)
}

func (s *charmServiceSuite) TestSetCharmRelationToReservedNameOnRequiresValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.charm.EXPECT().Actions().Return(&internalcharm.Actions{})
	s.charm.EXPECT().Config().Return(&internalcharm.Config{})

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name:        "foo",
		Subordinate: true,
		Requires: map[string]internalcharm.Relation{
			"juju-foo": {
				Name:      "juju-foo",
				Role:      internalcharm.RoleRequirer,
				Scope:     internalcharm.ScopeContainer,
				Interface: "juju-blah",
			},
		},
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	s.state.EXPECT().SetCharm(gomock.Any(), gomock.Any(), nil).Return(id, nil)

	got, warnings, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(warnings, gc.HasLen, 0)
	c.Check(got, gc.DeepEquals, id)
}

func (s *charmServiceSuite) TestSetCharmRelationToReservedNameOnRequiresInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
		Requires: map[string]internalcharm.Relation{
			"juju-foo": {
				Name:      "juju-foo",
				Role:      internalcharm.RoleRequirer,
				Scope:     internalcharm.ScopeGlobal,
				Interface: "blah",
			},
		},
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{Bases: []internalcharm.Base{{
		Name:          "ubuntu",
		Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
		Architectures: []string{"arm64"},
	}}}).MinTimes(1)

	_, _, err := s.service.SetCharm(context.Background(), charm.SetCharmArgs{
		Charm:         s.charm,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      1,
		Architecture:  arch.AMD64,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmRelationReservedNameMisuse)
}

func (s *charmServiceSuite) TestDeleteCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().DeleteCharm(gomock.Any(), id).Return(nil)

	err := s.service.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmServiceSuite) TestListCharmLocatorsWithName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expected := []charm.CharmLocator{{
		Name:         "foo",
		Source:       charm.LocalSource,
		Revision:     1,
		Architecture: architecture.AMD64,
	}}
	s.state.EXPECT().ListCharmLocatorsByNames(gomock.Any(), []string{"foo"}).Return(expected, nil)

	results, err := s.service.ListCharmLocators(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, 1)
	c.Check(results, gc.DeepEquals, expected)
}

func (s *charmServiceSuite) TestListCharmLocatorsWithoutName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// If no names are passed in, we call a different state method.
	// This simplifies the API for the caller, but makes the state methods
	// very easy to implement.

	expected := []charm.CharmLocator{{
		Name:         "foo",
		Source:       charm.LocalSource,
		Revision:     1,
		Architecture: architecture.AMD64,
	}}
	s.state.EXPECT().ListCharmLocators(gomock.Any()).Return(expected, nil)

	results, err := s.service.ListCharmLocators(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, 1)
	c.Check(results, gc.DeepEquals, expected)
}

func (s *charmServiceSuite) TestGetCharmDownloadInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	expected := &charm.DownloadInfo{
		Provenance:         charm.ProvenanceDownload,
		CharmhubIdentifier: "foo",
		DownloadURL:        "http://example.com/foo",
		DownloadSize:       42,
	}

	s.state.EXPECT().GetCharmDownloadInfo(gomock.Any(), id).Return(expected, nil)

	result, err := s.service.GetCharmDownloadInfo(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, expected)
}

func (s *charmServiceSuite) TestGetCharmDownloadInfoInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmDownloadInfo(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestGetAvailableCharmArchiveSHA256(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), id).Return("hash", nil)

	result, err := s.service.GetAvailableCharmArchiveSHA256(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, "hash")
}

func (s *charmServiceSuite) TestGetAvailableCharmArchiveSHA256InvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetAvailableCharmArchiveSHA256(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

type watchableServiceSuite struct {
	baseSuite

	watcherFactory *MockWatcherFactory

	service *WatchableService
}

var _ = gc.Suite(&watchableServiceSuite{})

func (s *watchableServiceSuite) TestWatchCharms(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string)
	stringsWatcher := watchertest.NewStringsWatcher(ch)
	defer workertest.DirtyKill(c, stringsWatcher)

	s.watcherFactory.EXPECT().NewUUIDsWatcher("charm", changestream.All).Return(stringsWatcher, nil)

	watcher, err := s.service.WatchCharms()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(watcher, gc.Equals, stringsWatcher)
}

func (s *watchableServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	s.service = &WatchableService{
		ProviderService: s.baseSuite.service,
		watcherFactory:  s.watcherFactory,
	}

	return ctrl
}
