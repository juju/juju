// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher/watchertest"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestGetMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := uuid.MustNewUUID().String()

	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: uuid.MustNewUUID().String(),
		SHA384: uuid.MustNewUUID().String(),
		Size:   666,
	}

	s.state.EXPECT().GetMetadata(gomock.Any(), path).Return(objectstore.Metadata{
		Path:   metadata.Path,
		Size:   metadata.Size,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
	}, nil)

	p, err := NewService(s.state).GetMetadata(c.Context(), path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(p, tc.DeepEquals, metadata)
}

func (s *serviceSuite) TestGetMetadataBySHA256(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sha256 := sha256.New()
	sha256.Write([]byte(uuid.MustNewUUID().String()))
	sha := hex.EncodeToString(sha256.Sum(nil))

	metadata := objectstore.Metadata{
		Path:   "path",
		SHA256: uuid.MustNewUUID().String(),
		SHA384: uuid.MustNewUUID().String(),
		Size:   666,
	}

	s.state.EXPECT().GetMetadataBySHA256(gomock.Any(), sha).Return(objectstore.Metadata{
		Path:   metadata.Path,
		Size:   metadata.Size,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
	}, nil)

	p, err := NewService(s.state).GetMetadataBySHA256(c.Context(), sha)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(p, tc.DeepEquals, metadata)
}

func (s *serviceSuite) TestGetMetadataBySHA256Invalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sha256 := sha256.New()
	sha256.Write([]byte(uuid.MustNewUUID().String()))
	sha := hex.EncodeToString(sha256.Sum(nil))

	illegalSha := "!" + sha[1:]

	_, err := NewService(s.state).GetMetadataBySHA256(c.Context(), illegalSha)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrInvalidHash)
}

func (s *serviceSuite) TestGetMetadataBySHA256TooShort(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sha256 := sha256.New()
	sha256.Write([]byte(uuid.MustNewUUID().String()))
	sha := hex.EncodeToString(sha256.Sum(nil))

	_, err := NewService(s.state).GetMetadataBySHA256(c.Context(), sha+"a")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrInvalidHashLength)
}

func (s *serviceSuite) TestGetMetadataBySHA256TooLong(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetMetadataBySHA256(c.Context(), "beef")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrInvalidHashLength)
}

func (s *serviceSuite) TestGetMetadataBySHA256Prefix(c *tc.C) {
	defer s.setupMocks(c).Finish()

	shaPrefix := "deadbeef"

	metadata := objectstore.Metadata{
		Path:   "path",
		SHA256: uuid.MustNewUUID().String(),
		SHA384: uuid.MustNewUUID().String(),
		Size:   666,
	}

	s.state.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), shaPrefix).Return(objectstore.Metadata{
		Path:   metadata.Path,
		Size:   metadata.Size,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
	}, nil)

	p, err := NewService(s.state).GetMetadataBySHA256Prefix(c.Context(), shaPrefix)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(p, tc.DeepEquals, metadata)
}

func (s *serviceSuite) TestGetMetadataBySHA256PrefixTooShort(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetMetadataBySHA256Prefix(c.Context(), "beef")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrHashPrefixTooShort)
}

func (s *serviceSuite) TestGetMetadataBySHA256PrefixTooLong(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetMetadataBySHA256Prefix(c.Context(), "deadbeef1")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrInvalidHashPrefix)
}

func (s *serviceSuite) TestGetMetadataBySHA256PrefixInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetMetadataBySHA256Prefix(c.Context(), "abcdefg")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrInvalidHashPrefix)
}

func (s *serviceSuite) TestListMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := uuid.MustNewUUID().String()

	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: uuid.MustNewUUID().String(),
		SHA384: uuid.MustNewUUID().String(),
		Size:   666,
	}

	s.state.EXPECT().ListMetadata(gomock.Any()).Return([]objectstore.Metadata{{
		Path:   metadata.Path,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
		Size:   metadata.Size,
	}}, nil)

	p, err := NewService(s.state).ListMetadata(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(p, tc.DeepEquals, []objectstore.Metadata{{
		Path:   metadata.Path,
		Size:   metadata.Size,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
	}})
}

func (s *serviceSuite) TestPutMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := uuid.MustNewUUID().String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: uuid.MustNewUUID().String(),
		SHA384: uuid.MustNewUUID().String(),
		Size:   666,
	}

	uuid := objectstore.GenUUID(c)
	s.state.EXPECT().PutMetadata(gomock.Any(), gomock.AssignableToTypeOf(objectstore.Metadata{})).DoAndReturn(func(ctx context.Context, data objectstore.Metadata) (objectstore.UUID, error) {
		c.Check(data.Path, tc.Equals, metadata.Path)
		c.Check(data.Size, tc.Equals, metadata.Size)
		c.Check(data.SHA256, tc.Equals, metadata.SHA256)
		c.Check(data.SHA384, tc.Equals, metadata.SHA384)
		return uuid, nil
	})

	result, err := NewService(s.state).PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, uuid)
}

func (s *serviceSuite) TestPutMetadataMissingSHA384(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := uuid.MustNewUUID().String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: uuid.MustNewUUID().String(),
		Size:   666,
	}

	_, err := NewService(s.state).PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrMissingHash)
}

func (s *serviceSuite) TestPutMetadataMissingSHA256(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := uuid.MustNewUUID().String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA384: uuid.MustNewUUID().String(),
		Size:   666,
	}

	_, err := NewService(s.state).PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrMissingHash)
}

// Test watch returns a watcher that watches the specified path.
func (s *serviceSuite) TestWatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	watcher := watchertest.NewMockStringsWatcher(nil)
	defer workertest.DirtyKill(c, watcher)

	table := "objectstore"
	stmt := "SELECT key FROM objectstore"
	s.state.EXPECT().InitialWatchStatement().Return(table, stmt)

	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(watcher, nil)

	w, err := NewWatchableService(s.state, s.watcherFactory).Watch(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	c.Cleanup(func() {
		s.state = nil
		s.watcherFactory = nil
	})

	return ctrl
}

type drainingServiceSuite struct {
	testhelpers.IsolationSuite

	state          *MockDrainingState
	watcherFactory *MockWatcherFactory
}

func TestDrainingServiceSuite(t *testing.T) {
	tc.Run(t, &drainingServiceSuite{})
}

func (s *drainingServiceSuite) TestSetDrainingPhase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentPhase := objectstore.PhaseUnknown
	newPhase := objectstore.PhaseDraining
	uuid := objectstore.GenUUID(c)

	s.state.EXPECT().GetActiveDrainingPhase(gomock.Any()).Return(uuid.String(), currentPhase, nil)
	s.state.EXPECT().SetDrainingPhase(gomock.Any(), uuid.String(), newPhase).Return(nil)

	err := NewWatchableDrainingService(s.state, s.watcherFactory).SetDrainingPhase(c.Context(), newPhase)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *drainingServiceSuite) TestSetDrainingPhaseNoInitial(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newPhase := objectstore.PhaseDraining

	s.state.EXPECT().GetActiveDrainingPhase(gomock.Any()).Return("", "", objectstoreerrors.ErrDrainingPhaseNotFound)
	s.state.EXPECT().SetDrainingPhase(gomock.Any(), gomock.Any(), newPhase).Return(nil)

	err := NewWatchableDrainingService(s.state, s.watcherFactory).SetDrainingPhase(c.Context(), newPhase)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *drainingServiceSuite) TestSetDrainingPhaseInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	phase := objectstore.Phase("invalid")

	err := NewWatchableDrainingService(s.state, s.watcherFactory).SetDrainingPhase(c.Context(), phase)
	c.Assert(err, tc.ErrorMatches, "invalid phase \"invalid\"")
}

func (s *drainingServiceSuite) TestSetDrainingPhaseError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentPhase := objectstore.PhaseUnknown
	newPhase := objectstore.PhaseDraining
	uuid := objectstore.GenUUID(c)

	s.state.EXPECT().GetActiveDrainingPhase(gomock.Any()).Return(uuid.String(), currentPhase, nil)
	s.state.EXPECT().SetDrainingPhase(gomock.Any(), uuid.String(), newPhase).Return(errors.Errorf("boom"))

	err := NewWatchableDrainingService(s.state, s.watcherFactory).SetDrainingPhase(c.Context(), newPhase)
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *drainingServiceSuite) TestGetDrainingPhase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	phase := objectstore.PhaseDraining
	uuid := objectstore.GenUUID(c)

	s.state.EXPECT().GetActiveDrainingPhase(gomock.Any()).Return(uuid.String(), phase, nil)

	p, err := NewWatchableDrainingService(s.state, s.watcherFactory).GetDrainingPhase(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(p, tc.Equals, phase)
}

func (s *drainingServiceSuite) TestGetDrainingPhaseError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	phase := objectstore.PhaseDraining
	uuid := objectstore.GenUUID(c)

	s.state.EXPECT().GetActiveDrainingPhase(gomock.Any()).Return(uuid.String(), phase, errors.Errorf("boom"))

	_, err := NewWatchableDrainingService(s.state, s.watcherFactory).GetDrainingPhase(c.Context())
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *drainingServiceSuite) TestRemoveMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := uuid.MustNewUUID().String()

	s.state.EXPECT().RemoveMetadata(gomock.Any(), key).Return(nil)

	err := NewWatchableDrainingService(s.state, s.watcherFactory).RemoveMetadata(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *drainingServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockDrainingState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	c.Cleanup(func() {
		s.state = nil
		s.watcherFactory = nil
	})

	return ctrl
}
