// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"
	"os"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	coreerrors "github.com/juju/juju/core/errors"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/charm/store"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/state/watcher/watchertest"
	"github.com/juju/juju/testcharms"
)

type charmServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&charmServiceSuite{})

func (s *charmServiceSuite) TestGetCharmIDWithoutRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.getCharmID(context.Background(), charm.GetCharmArgs{
		Name:   "foo",
		Source: charm.CharmHubSource,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmIDWithoutSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.getCharmID(context.Background(), charm.GetCharmArgs{
		Name:     "foo",
		Revision: ptr(42),
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmSourceNotValid)
}

func (s *charmServiceSuite) TestGetCharmIDInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.getCharmID(context.Background(), charm.GetCharmArgs{
		Name: "Foo",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *charmServiceSuite) TestGetCharmIDInvalidSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.getCharmID(context.Background(), charm.GetCharmArgs{
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

	result, err := s.service.getCharmID(context.Background(), charm.GetCharmArgs{
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

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().IsControllerCharm(gomock.Any(), id).Return(true, nil)

	result, err := s.service.IsControllerCharm(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *charmServiceSuite) TestIsControllerCharmCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().IsControllerCharm(gomock.Any(), id).Return(false, applicationerrors.CharmNotFound)

	_, err := s.service.IsControllerCharm(context.Background(), locator)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestIsCharmAvailable(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().IsCharmAvailable(gomock.Any(), id).Return(true, nil)

	result, err := s.service.IsCharmAvailable(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *charmServiceSuite) TestIsCharmAvailableCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().IsCharmAvailable(gomock.Any(), id).Return(false, applicationerrors.CharmNotFound)

	_, err := s.service.IsCharmAvailable(context.Background(), locator)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestSupportsContainers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().SupportsContainers(gomock.Any(), id).Return(true, nil)

	result, err := s.service.SupportsContainers(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *charmServiceSuite) TestSupportsContainersCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().SupportsContainers(gomock.Any(), id).Return(false, applicationerrors.CharmNotFound)

	_, err := s.service.SupportsContainers(context.Background(), locator)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Conversion of the metadata tests is done in the types package.

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmID(gomock.Any(), "foo", 42, charm.LocalSource).Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(charm.Charm{
		Metadata: charm.Metadata{
			Name: "foo",

			// RunAs becomes mandatory when being persisted. Empty string is not
			// allowed.
			RunAs: "default",
		},
		Source:    charm.LocalSource,
		Revision:  42,
		Available: true,
	}, nil, nil)

	metadata, locator, isAvailable, err := s.service.GetCharm(context.Background(), charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.LocalSource,
	})
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

	s.state.EXPECT().GetCharmID(gomock.Any(), "foo", 42, charm.LocalSource).Return(id, applicationerrors.CharmNotFound)

	_, _, _, err := s.service.GetCharm(context.Background(), charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.LocalSource,
	})

	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Conversion of the metadata tests is done in the types package.

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmMetadata(gomock.Any(), id).Return(charm.Metadata{
		Name: "foo",

		// RunAs becomes mandatory when being persisted. Empty string is not
		// allowed.
		RunAs: "default",
	}, nil)

	metadata, err := s.service.GetCharmMetadata(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata, gc.DeepEquals, internalcharm.Meta{
		Name: "foo",

		// Notice that the RunAs field becomes empty string when being returned.
	})
}

func (s *charmServiceSuite) TestGetCharmMetadataCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmMetadata(gomock.Any(), id).Return(charm.Metadata{}, applicationerrors.CharmNotFound)

	_, err := s.service.GetCharmMetadata(context.Background(), locator)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmLXDProfile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmLXDProfile(gomock.Any(), id).Return([]byte(`{"config": {"foo":"bar"}, "description": "description", "devices": {"gpu":{"baz": "x"}}}`), 42, nil)

	profile, revision, err := s.service.GetCharmLXDProfile(context.Background(), locator)
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

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmLXDProfile(gomock.Any(), id).Return(nil, -1, applicationerrors.CharmNotFound)

	_, _, err := s.service.GetCharmLXDProfile(context.Background(), locator)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmMetadataName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmMetadataName(gomock.Any(), id).Return("name for a charm", nil)

	name, err := s.service.GetCharmMetadataName(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, "name for a charm")
}

func (s *charmServiceSuite) TestGetCharmMetadataNameCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmMetadataName(gomock.Any(), id).Return("", applicationerrors.CharmNotFound)

	_, err := s.service.GetCharmMetadataName(context.Background(), locator)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmMetadataDescription(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmMetadataDescription(gomock.Any(), id).Return("description for a charm", nil)

	description, err := s.service.GetCharmMetadataDescription(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(description, gc.Equals, "description for a charm")
}

func (s *charmServiceSuite) TestGetCharmMetadataDescriptionCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmMetadataDescription(gomock.Any(), id).Return("", applicationerrors.CharmNotFound)

	_, err := s.service.GetCharmMetadataDescription(context.Background(), locator)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmMetadataStorage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), id).Return(map[string]charm.Storage{
		"foo": {
			Name:        "foo",
			Description: "description",
			Type:        charm.StorageBlock,
			Location:    "/foo",
		},
	}, nil)

	storage, err := s.service.GetCharmMetadataStorage(context.Background(), locator)
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

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmMetadataStorage(gomock.Any(), id).Return(nil, applicationerrors.CharmNotFound)

	_, err := s.service.GetCharmMetadataStorage(context.Background(), locator)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmMetadataResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmMetadataResources(gomock.Any(), id).Return(map[string]charm.Resource{
		"foo": {
			Name:        "foo",
			Type:        charm.ResourceTypeFile,
			Description: "description",
			Path:        "/foo",
		},
	}, nil)

	resources, err := s.service.GetCharmMetadataResources(context.Background(), locator)
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

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmMetadataResources(gomock.Any(), id).Return(nil, applicationerrors.CharmNotFound)

	_, err := s.service.GetCharmMetadataResources(context.Background(), locator)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestCharmManifest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmManifest(gomock.Any(), id).Return(s.minimalManifest(), nil)

	manifest, err := s.service.GetCharmManifest(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(manifest, gc.DeepEquals, internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name:          "ubuntu",
			Channel:       internalcharm.Channel{Risk: internalcharm.Stable},
			Architectures: []string{"amd64"},
		}},
	})
}

func (s *charmServiceSuite) TestCharmManifestInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmManifest(gomock.Any(), id).Return(charm.Manifest{
		Bases: []charm.Base{{}},
	}, nil)

	_, err := s.service.GetCharmManifest(context.Background(), locator)
	c.Assert(err, gc.ErrorMatches, "decode bases.*")
}

func (s *charmServiceSuite) TestGetCharmActions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmActions(gomock.Any(), id).Return(charm.Actions{
		Actions: map[string]charm.Action{
			"foo": {
				Description: "bar",
			},
		},
	}, nil)

	actions, err := s.service.GetCharmActions(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(actions, gc.DeepEquals, internalcharm.Actions{
		ActionSpecs: map[string]internalcharm.ActionSpec{
			"foo": {
				Description: "bar",
			},
		},
	})
}

func (s *charmServiceSuite) TestGetCharmConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmConfig(gomock.Any(), id).Return(charm.Config{
		Options: map[string]charm.Option{
			"foo": {
				Type: "string",
			},
		},
	}, nil)

	config, err := s.service.GetCharmConfig(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, gc.DeepEquals, internalcharm.Config{
		Options: map[string]internalcharm.Option{
			"foo": {
				Type: "string",
			},
		},
	})
}

func (s *charmServiceSuite) TestGetCharmArchivePath(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmArchivePath(gomock.Any(), id).Return("archive-path", nil)

	path, err := s.service.GetCharmArchivePath(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "archive-path")
}

func (s *charmServiceSuite) TestGetCharmArchivePathCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmArchivePath(gomock.Any(), id).Return("archive-path", applicationerrors.CharmNotFound)

	_, err := s.service.GetCharmArchivePath(context.Background(), locator)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmArchive(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)
	archive := io.NopCloser(strings.NewReader("archive-content"))

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmArchiveMetadata(gomock.Any(), id).Return("archive-path", "hash", nil)
	s.charmStore.EXPECT().Get(gomock.Any(), "archive-path").Return(archive, nil)

	reader, hash, err := s.service.GetCharmArchive(context.Background(), locator)
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

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmArchiveMetadata(gomock.Any(), id).Return("", "", applicationerrors.CharmNotFound)

	_, _, err := s.service.GetCharmArchive(context.Background(), locator)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestSetCharmAvailable(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().SetCharmAvailable(gomock.Any(), id).Return(nil)

	err := s.service.SetCharmAvailable(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmServiceSuite) TestSetCharmAvailableCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().SetCharmAvailable(gomock.Any(), id).Return(applicationerrors.CharmNotFound)

	err := s.service.SetCharmAvailable(context.Background(), locator)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
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
		Manifest:      s.minimalManifest(),
		ReferenceName: "baz",
		Source:        charm.LocalSource,
		Revision:      1,
		Architecture:  architecture.AMD64,
	}, downloadInfo, false).Return(id, charm.CharmLocator{
		Name:         "foo",
		Revision:     1,
		Source:       charm.LocalSource,
		Architecture: architecture.AMD64,
	}, nil)

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
		Manifest:      s.minimalManifest(),
		ReferenceName: "baz",
		Source:        charm.CharmHubSource,
		Revision:      1,
		Architecture:  architecture.AMD64,
	}, downloadInfo, false).Return(id, charm.CharmLocator{
		Name:         "foo",
		Revision:     1,
		Source:       charm.LocalSource,
		Architecture: architecture.AMD64,
	}, nil)

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

	s.state.EXPECT().SetCharm(gomock.Any(), gomock.Any(), nil, false).Return(id, charm.CharmLocator{
		Name:         "foo",
		Revision:     1,
		Source:       charm.LocalSource,
		Architecture: architecture.AMD64,
	}, nil)

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

	s.state.EXPECT().SetCharm(gomock.Any(), gomock.Any(), nil, false).Return(id, charm.CharmLocator{
		Name:         "foo",
		Revision:     1,
		Source:       charm.LocalSource,
		Architecture: architecture.AMD64,
	}, nil)

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

	s.state.EXPECT().SetCharm(gomock.Any(), gomock.Any(), nil, false).Return(id, charm.CharmLocator{
		Name:         "foo",
		Revision:     1,
		Source:       charm.LocalSource,
		Architecture: architecture.AMD64,
	}, nil)

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

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().DeleteCharm(gomock.Any(), id).Return(nil)

	err := s.service.DeleteCharm(context.Background(), locator)
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

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetCharmDownloadInfo(gomock.Any(), id).Return(expected, nil)

	result, err := s.service.GetCharmDownloadInfo(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, expected)
}

func (s *charmServiceSuite) TestGetAvailableCharmArchiveSHA256(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	locator := charm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   charm.CharmHubSource,
	}
	s.state.EXPECT().GetCharmID(gomock.Any(), locator.Name, locator.Revision, locator.Source).Return(id, nil)
	s.state.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), id).Return("hash", nil)

	result, err := s.service.GetAvailableCharmArchiveSHA256(context.Background(), locator)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, "hash")
}

func (s *charmServiceSuite) TestResolveUploadCharmInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.ResolveUploadCharm(context.Background(), charm.ResolveUploadCharm{
		Source: "",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmSourceNotValid)
}

func (s *charmServiceSuite) TestResolveUploadCharmCharmhubNotDuringNotImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Charmhub charms are not allowed to be uploaded whilst the model is
	// not importing.

	_, err := s.service.ResolveUploadCharm(context.Background(), charm.ResolveUploadCharm{
		Source:    corecharm.CharmHub,
		Importing: false,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.NonLocalCharmImporting)
}

func (s *charmServiceSuite) TestResolveUploadCharmLocalCharmNotImporting(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// SetCharm is tested in tests elsewhere, so we can just return a valid
	// charm ID here.

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")
	file, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	stat, err := file.Stat()
	c.Assert(err, jc.ErrorIsNil)

	charmID := charmtesting.GenCharmID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	downloadInfo := &charm.DownloadInfo{
		Provenance: charm.ProvenanceUpload,
	}

	s.charmStore.EXPECT().StoreFromReader(gomock.Any(), gomock.Not(gomock.Nil()), "abc").
		DoAndReturn(func(ctx context.Context, r io.Reader, s string) (store.StoreFromReaderResult, store.Digest, error) {
			_, err := file.Seek(0, io.SeekStart)
			c.Assert(err, jc.ErrorIsNil)

			return store.StoreFromReaderResult{
					Charm:           file,
					ObjectStoreUUID: objectStoreUUID,
					UniqueName:      "unique-name",
				}, store.Digest{
					SHA256: "sha-256",
					SHA384: "sha-384",
					Size:   stat.Size(),
				}, nil
		})
	s.state.EXPECT().SetCharm(gomock.Any(), gomock.Any(), downloadInfo, true).DoAndReturn(func(_ context.Context, ch charm.Charm, _ *charm.DownloadInfo, _ bool) (corecharm.ID, charm.CharmLocator, error) {
		c.Check(ch.Metadata.Name, gc.Equals, "dummy")
		return charmID, charm.CharmLocator{
			Name:         "test",
			Revision:     1,
			Source:       charm.LocalSource,
			Architecture: architecture.AMD64,
		}, nil
	})

	locator, err := s.service.ResolveUploadCharm(context.Background(), charm.ResolveUploadCharm{
		Source:       corecharm.Local,
		Reader:       file,
		SHA256Prefix: "abc",
		Architecture: arch.AMD64,
		Revision:     -1,
		Name:         "test",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(locator, gc.DeepEquals, charm.CharmLocator{
		Name:         "test",
		Revision:     1,
		Source:       charm.LocalSource,
		Architecture: architecture.AMD64,
	})
}

func (s *charmServiceSuite) TestResolveUploadCharmLocalCharmNotImportingFailedRead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	s.charmStore.EXPECT().StoreFromReader(gomock.Any(), gomock.Not(gomock.Nil()), "abc").Return(store.StoreFromReaderResult{
		ObjectStoreUUID: objectStoreUUID,
		UniqueName:      "unique-name",
	}, store.Digest{
		SHA256: "sha-256",
		SHA384: "sha-384",
		Size:   42,
	}, errors.Errorf("failed to read %w", coreerrors.NotValid))

	_, err := s.service.ResolveUploadCharm(context.Background(), charm.ResolveUploadCharm{
		Source:       corecharm.Local,
		Reader:       strings.NewReader("foo"),
		SHA256Prefix: "abc",
		Architecture: arch.AMD64,
		Revision:     1,
		Name:         "test",
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *charmServiceSuite) TestResolveUploadCharmLocalCharmNotImportingFailedSetCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")
	file, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	stat, err := file.Stat()
	c.Assert(err, jc.ErrorIsNil)

	charmID := charmtesting.GenCharmID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	downloadInfo := &charm.DownloadInfo{
		Provenance: charm.ProvenanceUpload,
	}

	s.charmStore.EXPECT().StoreFromReader(gomock.Any(), gomock.Not(gomock.Nil()), "abc").
		DoAndReturn(func(ctx context.Context, r io.Reader, s string) (store.StoreFromReaderResult, store.Digest, error) {
			_, err := file.Seek(0, io.SeekStart)
			c.Assert(err, jc.ErrorIsNil)

			return store.StoreFromReaderResult{
					Charm:           file,
					ObjectStoreUUID: objectStoreUUID,
					UniqueName:      "unique-name",
				}, store.Digest{
					SHA256: "sha-256",
					SHA384: "sha-384",
					Size:   stat.Size(),
				}, nil
		})
	s.state.EXPECT().SetCharm(gomock.Any(), gomock.Any(), downloadInfo, true).DoAndReturn(func(_ context.Context, _ charm.Charm, _ *charm.DownloadInfo, _ bool) (corecharm.ID, charm.CharmLocator, error) {
		return charmID, charm.CharmLocator{}, errors.Errorf("failed to set charm %w", coreerrors.NotValid)
	})

	_, err = s.service.ResolveUploadCharm(context.Background(), charm.ResolveUploadCharm{
		Source:       corecharm.Local,
		Reader:       file,
		SHA256Prefix: "abc",
		Architecture: arch.AMD64,
		Revision:     -1,
		Name:         "test",
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *charmServiceSuite) TestResolveUploadCharmLocalCharmImporting(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// SetCharm is tested in tests elsewhere, so we can just return a valid
	// charm ID here.

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")
	file, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	stat, err := file.Stat()
	c.Assert(err, jc.ErrorIsNil)

	charmID := charmtesting.GenCharmID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	downloadInfo := &charm.DownloadInfo{
		Provenance: charm.ProvenanceMigration,
	}

	s.state.EXPECT().GetCharmID(gomock.Any(), "test", 1, charm.LocalSource).Return(charmID, nil)
	s.charmStore.EXPECT().StoreFromReader(gomock.Any(), gomock.Not(gomock.Nil()), "abc").
		DoAndReturn(func(ctx context.Context, r io.Reader, s string) (store.StoreFromReaderResult, store.Digest, error) {
			_, err := file.Seek(0, io.SeekStart)
			c.Assert(err, jc.ErrorIsNil)

			return store.StoreFromReaderResult{
					Charm:           file,
					ObjectStoreUUID: objectStoreUUID,
					UniqueName:      "unique-name",
				}, store.Digest{
					SHA256: "sha-256",
					SHA384: "sha-384",
					Size:   stat.Size(),
				}, nil
		})
	s.state.EXPECT().ResolveMigratingUploadedCharm(gomock.Any(), charmID, charm.ResolvedMigratingUploadedCharm{
		ObjectStoreUUID: objectStoreUUID,
		Hash:            "sha-256",
		ArchivePath:     "unique-name",
		DownloadInfo:    downloadInfo,
	}).Return(charm.CharmLocator{
		Name:         "test",
		Revision:     1,
		Source:       charm.LocalSource,
		Architecture: architecture.AMD64,
	}, nil)

	locator, err := s.service.ResolveUploadCharm(context.Background(), charm.ResolveUploadCharm{
		Source:       corecharm.Local,
		Reader:       file,
		SHA256Prefix: "abc",
		Architecture: arch.AMD64,
		Revision:     1,
		Name:         "test",
		Importing:    true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(locator, gc.DeepEquals, charm.CharmLocator{
		Name:         "test",
		Revision:     1,
		Source:       charm.LocalSource,
		Architecture: architecture.AMD64,
	})
}

func (s *charmServiceSuite) TestResolveUploadCharmLocalCharmImportingCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")
	reader, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)

	charmID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmID(gomock.Any(), "test", 1, charm.LocalSource).Return(charmID, applicationerrors.CharmNotFound)

	_, err = s.service.ResolveUploadCharm(context.Background(), charm.ResolveUploadCharm{
		Source:       corecharm.Local,
		Reader:       reader,
		SHA256Prefix: "abc",
		Architecture: arch.AMD64,
		Revision:     1,
		Name:         "test",
		Importing:    true,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestResolveUploadCharmLocalCharmImportingFailedStore(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")
	reader, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	stat, err := reader.Stat()
	c.Assert(err, jc.ErrorIsNil)

	charmID := charmtesting.GenCharmID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	s.state.EXPECT().GetCharmID(gomock.Any(), "test", 1, charm.LocalSource).Return(charmID, nil)
	s.charmStore.EXPECT().StoreFromReader(gomock.Any(), gomock.Not(gomock.Nil()), "abc").Return(store.StoreFromReaderResult{
		ObjectStoreUUID: objectStoreUUID,
		UniqueName:      "unique-name",
	}, store.Digest{
		SHA256: "sha-256",
		SHA384: "sha-384",
		Size:   stat.Size(),
	}, errors.Errorf("failed to store %w", coreerrors.NotValid))

	_, err = s.service.ResolveUploadCharm(context.Background(), charm.ResolveUploadCharm{
		Source:       corecharm.Local,
		Reader:       reader,
		SHA256Prefix: "abc",
		Architecture: arch.AMD64,
		Revision:     1,
		Name:         "test",
		Importing:    true,
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *charmServiceSuite) TestResolveUploadCharmLocalCharmImportingFailedResolve(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")
	file, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	stat, err := file.Stat()
	c.Assert(err, jc.ErrorIsNil)

	charmID := charmtesting.GenCharmID(c)
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	downloadInfo := &charm.DownloadInfo{
		Provenance: charm.ProvenanceMigration,
	}

	s.state.EXPECT().GetCharmID(gomock.Any(), "test", 1, charm.LocalSource).Return(charmID, nil)
	s.charmStore.EXPECT().StoreFromReader(gomock.Any(), gomock.Not(gomock.Nil()), "abc").
		DoAndReturn(func(ctx context.Context, r io.Reader, s string) (store.StoreFromReaderResult, store.Digest, error) {
			_, err := file.Seek(0, io.SeekStart)
			c.Assert(err, jc.ErrorIsNil)

			return store.StoreFromReaderResult{
					Charm:           file,
					ObjectStoreUUID: objectStoreUUID,
					UniqueName:      "unique-name",
				}, store.Digest{
					SHA256: "sha-256",
					SHA384: "sha-384",
					Size:   stat.Size(),
				}, nil
		})
	s.state.EXPECT().ResolveMigratingUploadedCharm(gomock.Any(), charmID, charm.ResolvedMigratingUploadedCharm{
		ObjectStoreUUID: objectStoreUUID,
		Hash:            "sha-256",
		ArchivePath:     "unique-name",
		DownloadInfo:    downloadInfo,
	}).Return(charm.CharmLocator{
		Name:         "test",
		Revision:     1,
		Source:       charm.LocalSource,
		Architecture: architecture.AMD64,
	}, errors.Errorf("failed to resolve %w", coreerrors.NotValid))

	_, err = s.service.ResolveUploadCharm(context.Background(), charm.ResolveUploadCharm{
		Source:       corecharm.Local,
		Reader:       file,
		SHA256Prefix: "abc",
		Architecture: arch.AMD64,
		Revision:     1,
		Name:         "test",
		Importing:    true,
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *charmServiceSuite) TestReserveCharmRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()

	metadata := &internalcharm.Meta{
		Name: "foo",
	}
	manifest := &internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name:          "ubuntu",
			Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
			Architectures: []string{"arm64"},
		}},
	}
	config := &internalcharm.Config{}
	actions := &internalcharm.Actions{}
	lxdProfile := &internalcharm.LXDProfile{}

	downloadInfo := &charm.DownloadInfo{
		Provenance:         charm.ProvenanceDownload,
		CharmhubIdentifier: "foo",
		DownloadURL:        "http://example.com/foo",
		DownloadSize:       42,
	}

	charmBase := internalcharm.NewCharmBase(metadata, manifest, config, actions, lxdProfile)
	ch, _, err := encodeCharm(charmBase)
	c.Assert(err, jc.ErrorIsNil)

	ch.Source = charm.CharmHubSource
	ch.ReferenceName = "foo"
	ch.Revision = 1
	ch.Hash = "hash"
	ch.Architecture = architecture.AMD64

	s.state.EXPECT().SetCharm(gomock.Any(), ch, downloadInfo, false).Return(corecharm.ID("id"), charm.CharmLocator{}, nil)

	_, _, err = s.service.ReserveCharmRevision(context.Background(), charm.ReserveCharmRevisionArgs{
		Charm:         charmBase,
		Source:        corecharm.CharmHub,
		ReferenceName: "foo",
		Revision:      1,
		Hash:          "hash",
		Architecture:  arch.AMD64,
		DownloadInfo:  downloadInfo,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmServiceSuite) TestReserveCharmRevisionAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()
	metadata := &internalcharm.Meta{
		Name: "foo",
	}
	manifest := &internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name:          "ubuntu",
			Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
			Architectures: []string{"arm64"},
		}},
	}
	config := &internalcharm.Config{}
	actions := &internalcharm.Actions{}
	lxdProfile := &internalcharm.LXDProfile{}

	charmBase := internalcharm.NewCharmBase(metadata, manifest, config, actions, lxdProfile)

	// We don't check the expected, we only care about the result
	s.state.EXPECT().SetCharm(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("id", charm.CharmLocator{},
		applicationerrors.CharmAlreadyExists)
	s.state.EXPECT().GetCharmID(gomock.Any(), charmBase.Meta().Name, 42, charm.LocalSource).Return("id", nil)

	id, _, err := s.service.ReserveCharmRevision(context.Background(), charm.ReserveCharmRevisionArgs{
		// We only define required fields to pass code before SetCharm call
		Charm:         charmBase,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      42,
	})
	c.Assert(id, gc.Equals, corecharm.ID("id"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmServiceSuite) TestReserveCharmRevisionAlreadyExistsGetCharmIdError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	metadata := &internalcharm.Meta{
		Name: "foo",
	}
	manifest := &internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name:          "ubuntu",
			Channel:       internalcharm.Channel{Risk: internalcharm.Beta},
			Architectures: []string{"arm64"},
		}},
	}
	config := &internalcharm.Config{}
	actions := &internalcharm.Actions{}
	lxdProfile := &internalcharm.LXDProfile{}

	charmBase := internalcharm.NewCharmBase(metadata, manifest, config, actions, lxdProfile)
	expectedError := errors.New("boom")

	// We don't check the expected, we only care about the result
	s.state.EXPECT().SetCharm(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("id", charm.CharmLocator{},
		applicationerrors.CharmAlreadyExists)
	s.state.EXPECT().GetCharmID(gomock.Any(), charmBase.Meta().Name, 42, charm.LocalSource).Return("", expectedError)

	_, _, err := s.service.ReserveCharmRevision(context.Background(), charm.ReserveCharmRevisionArgs{
		// We only define required fields to pass code before SetCharm call
		Charm:         charmBase,
		Source:        corecharm.Local,
		ReferenceName: "foo",
		Revision:      42,
	})
	c.Assert(err, jc.ErrorIs, expectedError)
}

func (s *charmServiceSuite) TestGetLatestPendingCharmhubCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expectedLocator := charm.CharmLocator{
		Name:         "foo",
		Revision:     42,
		Source:       charm.CharmHubSource,
		Architecture: architecture.AMD64,
	}
	s.state.EXPECT().GetLatestPendingCharmhubCharm(gomock.Any(), "foo", architecture.AMD64).Return(expectedLocator, nil)

	result, err := s.service.GetLatestPendingCharmhubCharm(context.Background(), "foo", arch.AMD64)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, expectedLocator)
}

func (s *charmServiceSuite) TestGetLatestPendingCharmhubCharmInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetLatestPendingCharmhubCharm(context.Background(), "!!!foo", arch.AMD64)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
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

	s.state.EXPECT().NamespaceForWatchCharm().Return("charm")
	s.watcherFactory.EXPECT().NewUUIDsWatcher("charm", changestream.All).Return(stringsWatcher, nil)

	watcher, err := s.service.WatchCharms()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(watcher, gc.Equals, stringsWatcher)
}

func (s *watchableServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.watcherFactory = NewMockWatcherFactory(ctrl)

	s.service = &WatchableService{
		ProviderService: s.baseSuite.service,
		watcherFactory:  s.watcherFactory,
	}

	return ctrl
}
