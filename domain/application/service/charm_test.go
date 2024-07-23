// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

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

	state *MockCharmState
	charm *MockCharm

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
		Source:   domaincharm.LocalSource,
		Revision: 1,
	}).Return(id, nil)

	got, warnings, err := s.service.SetCharm(context.Background(), domaincharm.SetCharmArgs{
		Charm:    s.charm,
		Source:   internalcharm.Local,
		Revision: 1,
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
		Charm:    s.charm,
		Source:   "charmstore",
		Revision: 1,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmSourceNotValid)
}

func (s *charmServiceSuite) TestDeleteCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().DeleteCharm(gomock.Any(), id).Return(nil)

	err := s.service.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockCharmState(ctrl)
	s.charm = NewMockCharm(ctrl)

	s.service = NewCharmService(s.state, loggertesting.WrapCheckLog(c))

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

	s.service = NewWatchableCharmService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))

	return ctrl
}
