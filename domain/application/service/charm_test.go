// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	charmtesting "github.com/juju/juju/core/charm/testing"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/state/watcher/watchertest"
)

type charmServiceSuite struct {
	testing.IsolationSuite

	state             *MockCharmState
	charm             *MockCharm
	objectStore       *MockObjectStore
	objectStoreGetter *MockModelObjectStoreGetter

	service *CharmService
}

var _ = gc.Suite(&charmServiceSuite{})

func (s *charmServiceSuite) TestGetCharmID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(context.Background(), domaincharm.GetCharmArgs{
		Name: "foo",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmIDInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(context.Background(), domaincharm.GetCharmArgs{
		Name: "Foo",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *charmServiceSuite) TestGetCharmIDWithRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	rev := 42

	s.state.EXPECT().GetCharmIDByRevision(gomock.Any(), "foo", rev).Return(id, nil)

	result, err := s.service.GetCharmID(context.Background(), domaincharm.GetCharmArgs{
		Name:     "foo",
		Revision: &rev,
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

	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name: "foo",

			// RunAs becomes mandatory when being persisted. Empty string is not
			// allowed.
			RunAs: "default",
		},
	}, domaincharm.CharmOrigin{
		Source:   domaincharm.LocalSource,
		Revision: 42,
	}, nil)

	metadata, origin, err := s.service.GetCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata.Meta(), gc.DeepEquals, &internalcharm.Meta{
		Name: "foo",

		// Notice that the RunAs field becomes empty string when being returned.
	})
	c.Check(origin, gc.Equals, domaincharm.CharmOrigin{
		Source:   domaincharm.LocalSource,
		Revision: 42,
	})
}

func (s *charmServiceSuite) TestGetCharmCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{}, domaincharm.CharmOrigin{}, applicationerrors.CharmNotFound)

	_, _, err := s.service.GetCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestGetCharmInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.GetCharm(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestGetCharmFromSha256ErrorDB(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetCharmArchivePathFromSha256(gomock.Any(), "deadbeef").Return("", false, errors.New("boom!"))

	_, err := s.service.GetCharmFromSha256(context.Background(), "deadbeef")
	c.Assert(err, gc.ErrorMatches, "boom!")
}

func (s *charmServiceSuite) TestGetCharmFromSha256NotAvailable(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetCharmArchivePathFromSha256(gomock.Any(), "deadbeef").Return("archive-path", false, nil)

	_, err := s.service.GetCharmFromSha256(context.Background(), "deadbeef")
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotYetAvailable)
}

func (s *charmServiceSuite) TestGetCharmFromSha256ErrorObjectStore(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetCharmArchivePathFromSha256(gomock.Any(), "deadbeef").Return("archive-path", true, nil)

	s.objectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(nil, errors.New("boom!"))

	_, err := s.service.GetCharmFromSha256(context.Background(), "deadbeef")
	c.Assert(err, gc.ErrorMatches, "accessing model object store: boom!")
}

func (s *charmServiceSuite) TestGetCharmFromSha256ErrorReadingCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetCharmArchivePathFromSha256(gomock.Any(), "deadbeef").Return("archive-path", true, nil)
	s.objectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.objectStore, nil)
	s.objectStore.EXPECT().Get(gomock.Any(), "archive-path").Return(nil, 0, errors.New("boom!"))

	_, err := s.service.GetCharmFromSha256(context.Background(), "deadbeef")
	c.Assert(err, gc.ErrorMatches, "retrieving charm from model object storage: boom!")
}

func (s *charmServiceSuite) TestGetCharmFromSha256Success(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetCharmArchivePathFromSha256(gomock.Any(), "deadbeef").Return("archive-path", true, nil)
	s.objectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.objectStore, nil)
	s.objectStore.EXPECT().Get(gomock.Any(), "archive-path").Return(io.NopCloser(strings.NewReader("charm-contents")), 14, nil)

	reader, err := s.service.GetCharmFromSha256(context.Background(), "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reader, gc.NotNil)

	buf := make([]byte, 1024)
	n, err := reader.Read(buf)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 14)
	c.Assert(buf[:n], gc.DeepEquals, []byte("charm-contents"))
}

func (s *charmServiceSuite) TestGetCharmMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Conversion of the metadata tests is done in the types package.

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmMetadata(gomock.Any(), id).Return(domaincharm.Metadata{
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

	s.state.EXPECT().GetCharmMetadata(gomock.Any(), id).Return(domaincharm.Metadata{}, applicationerrors.CharmNotFound)

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

func (s *charmServiceSuite) TestReserveCharmRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id1 := charmtesting.GenCharmID(c)
	id2 := charmtesting.GenCharmID(c)

	s.state.EXPECT().ReserveCharmRevision(gomock.Any(), id1, 21).Return(id2, nil)

	result, err := s.service.ReserveCharmRevision(context.Background(), id1, 21)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, id2)
}

func (s *charmServiceSuite) TestReserveCharmRevisionCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id1 := charmtesting.GenCharmID(c)
	id2 := charmtesting.GenCharmID(c)

	s.state.EXPECT().ReserveCharmRevision(gomock.Any(), id1, 21).Return(id2, applicationerrors.CharmNotFound)

	_, err := s.service.ReserveCharmRevision(context.Background(), id1, 21)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmServiceSuite) TestReserveCharmRevisionInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.ReserveCharmRevision(context.Background(), "", 21)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *charmServiceSuite) TestReserveCharmRevisionInvalidRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	_, err := s.service.ReserveCharmRevision(context.Background(), id, -1)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmRevisionNotValid)
}

func (s *charmServiceSuite) TestSetCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
	}).Times(2)
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{})
	s.charm.EXPECT().Actions().Return(&internalcharm.Actions{})
	s.charm.EXPECT().Config().Return(&internalcharm.Config{})

	s.state.EXPECT().SetCharm(gomock.Any(), domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
		},
	}, domaincharm.SetStateArgs{
		ReferenceName: "baz",
		Source:        domaincharm.LocalSource,
		Revision:      1,
	}).Return(id, nil)

	got, warnings, err := s.service.SetCharm(context.Background(), domaincharm.SetCharmArgs{
		Charm:         s.charm,
		Source:        internalcharm.Local,
		ReferenceName: "baz",
		Revision:      1,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(warnings, gc.HasLen, 0)
	c.Check(got, gc.DeepEquals, id)
}

func (s *charmServiceSuite) TestSetCharmNoName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{})

	_, _, err := s.service.SetCharm(context.Background(), domaincharm.SetCharmArgs{
		Charm:    s.charm,
		Source:   internalcharm.Local,
		Revision: 1,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *charmServiceSuite) TestSetCharmInvalidSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
	})

	_, _, err := s.service.SetCharm(context.Background(), domaincharm.SetCharmArgs{
		Charm:         s.charm,
		Source:        "charmstore",
		ReferenceName: "foo",
		Revision:      1,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmSourceNotValid)
}

func (s *charmServiceSuite) TestSetCharmRelationNameConflict(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name: "foo",
		Provides: map[string]internalcharm.Relation{
			"foo": {
				Role:  internalcharm.RoleProvider,
				Scope: internalcharm.ScopeGlobal,
			},
		},
		Requires: map[string]internalcharm.Relation{
			"foo": {
				Role:  internalcharm.RoleRequirer,
				Scope: internalcharm.ScopeGlobal,
			},
		},
	}).Times(2)

	_, _, err := s.service.SetCharm(context.Background(), domaincharm.SetCharmArgs{
		Charm:         s.charm,
		Source:        internalcharm.Local,
		ReferenceName: "foo",
		Revision:      1,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmRelationNameConflict)
}

func (s *charmServiceSuite) TestDeleteCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().DeleteCharm(gomock.Any(), id).Return(nil)

	err := s.service.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmServiceSuite) TestListAllCharms(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expected := []domaincharm.CharmWithOrigin{{
		Name: "foo",
		CharmOrigin: domaincharm.CharmOrigin{
			ReferenceName: "foo",
			Source:        domaincharm.LocalSource,
			Revision:      1,
			Platform: domaincharm.Platform{
				Architecture: domaincharm.ARM64,
			},
		},
	}}
	s.state.EXPECT().ListCharmsWithOriginByNames(gomock.Any(), []string{"foo"}).Return(expected, nil)

	results, err := s.service.ListCharmsWithOriginByNames(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, 1)
	c.Check(results, gc.DeepEquals, expected)
}

func (s *charmServiceSuite) TestListAllCharmsByNames(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// If no names are passed in, we call a different state method.
	// This simplifies the API for the caller, but makes the state methods
	// very easy to implement.

	expected := []domaincharm.CharmWithOrigin{{
		Name: "foo",
		CharmOrigin: domaincharm.CharmOrigin{
			ReferenceName: "foo",
			Source:        domaincharm.LocalSource,
			Revision:      1,
			Platform: domaincharm.Platform{
				Architecture: domaincharm.ARM64,
			},
		},
	}}
	s.state.EXPECT().ListCharmsWithOrigin(gomock.Any()).Return(expected, nil)

	results, err := s.service.ListCharmsWithOriginByNames(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, 1)
	c.Check(results, gc.DeepEquals, expected)
}

func (s *charmServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockCharmState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)
	s.objectStoreGetter = NewMockModelObjectStoreGetter(ctrl)

	s.service = NewCharmService(s.state, s.objectStoreGetter, loggertesting.WrapCheckLog(c))

	return ctrl
}

type watchableServiceSuite struct {
	testing.IsolationSuite

	state          *MockCharmState
	watcherFactory *MockWatcherFactory

	service *WatchableCharmService
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
	ctrl := gomock.NewController(c)

	s.state = NewMockCharmState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	s.service = NewWatchableCharmService(s.state, s.watcherFactory, nil, loggertesting.WrapCheckLog(c))

	return ctrl
}
