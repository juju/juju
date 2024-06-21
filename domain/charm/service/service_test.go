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
	domaincharm "github.com/juju/juju/domain/charm"
	charmerrors "github.com/juju/juju/domain/charm/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/state/watcher/watchertest"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState

	service *Service
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestGetCharmID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByLatestRevision(gomock.Any(), "foo").Return(id, nil)

	result, err := s.service.GetCharmID(context.Background(), domaincharm.GetCharmArgs{
		Name: "foo",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, id)
}

func (s *serviceSuite) TestGetCharmIDInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(context.Background(), domaincharm.GetCharmArgs{
		Name: "Foo",
	})
	c.Assert(err, jc.ErrorIs, charmerrors.NameNotValid)
}

func (s *serviceSuite) TestGetCharmIDWithRevision(c *gc.C) {
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

func (s *serviceSuite) TestGetCharmIDErrorNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetCharmIDByLatestRevision(gomock.Any(), "foo").Return("", charmerrors.NotFound)

	_, err := s.service.GetCharmID(context.Background(), domaincharm.GetCharmArgs{
		Name: "foo",
	})
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *serviceSuite) TestIsControllerCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().IsControllerCharm(gomock.Any(), id).Return(true, nil)

	result, err := s.service.IsControllerCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *serviceSuite) TestIsControllerCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().IsControllerCharm(gomock.Any(), id).Return(false, charmerrors.NotFound)

	_, err := s.service.IsControllerCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *serviceSuite) TestIsControllerCharmInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.IsControllerCharm(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestIsCharmAvailable(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().IsCharmAvailable(gomock.Any(), id).Return(true, nil)

	result, err := s.service.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *serviceSuite) TestIsCharmAvailableNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().IsCharmAvailable(gomock.Any(), id).Return(false, charmerrors.NotFound)

	_, err := s.service.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *serviceSuite) TestIsCharmAvailableInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.IsCharmAvailable(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestSupportsContainers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().SupportsContainers(gomock.Any(), id).Return(true, nil)

	result, err := s.service.SupportsContainers(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *serviceSuite) TestSupportsContainersNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().SupportsContainers(gomock.Any(), id).Return(false, charmerrors.NotFound)

	_, err := s.service.SupportsContainers(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *serviceSuite) TestSupportsContainersInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.SupportsContainers(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestGetCharmMetadata(c *gc.C) {
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

func (s *serviceSuite) TestGetCharmMetadataNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmMetadata(gomock.Any(), id).Return(domaincharm.Metadata{}, charmerrors.NotFound)

	_, err := s.service.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *serviceSuite) TestGetCharmMetadataInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmMetadata(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestSetCharmAvailable(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().SetCharmAvailable(gomock.Any(), id).Return(nil)

	err := s.service.SetCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetCharmAvailableNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().SetCharmAvailable(gomock.Any(), id).Return(charmerrors.NotFound)

	err := s.service.SetCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *serviceSuite) TestSetCharmAvailableInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetCharmAvailable(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestReserveCharmRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id1 := charmtesting.GenCharmID(c)
	id2 := charmtesting.GenCharmID(c)

	s.state.EXPECT().ReserveCharmRevision(gomock.Any(), id1, 21).Return(id2, nil)

	result, err := s.service.ReserveCharmRevision(context.Background(), id1, 21)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, id2)
}

func (s *serviceSuite) TestReserveCharmRevisionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id1 := charmtesting.GenCharmID(c)
	id2 := charmtesting.GenCharmID(c)

	s.state.EXPECT().ReserveCharmRevision(gomock.Any(), id1, 21).Return(id2, charmerrors.NotFound)

	_, err := s.service.ReserveCharmRevision(context.Background(), id1, 21)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *serviceSuite) TestReserveCharmRevisionInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.ReserveCharmRevision(context.Background(), "", 21)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestReserveCharmRevisionInvalidRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	_, err := s.service.ReserveCharmRevision(context.Background(), id, -1)
	c.Assert(err, jc.ErrorIs, charmerrors.RevisionNotValid)
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	s.service = NewService(s.state, loggertesting.WrapCheckLog(c))

	return ctrl
}

type watchableServiceSuite struct {
	testing.IsolationSuite

	state          *MockState
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
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	s.service = NewWatchableService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))

	return ctrl
}
