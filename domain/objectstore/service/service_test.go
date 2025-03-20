// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testing.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestGetMetadata(c *gc.C) {
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

	p, err := NewService(s.state).GetMetadata(context.Background(), path)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(p, gc.DeepEquals, metadata)
}

func (s *serviceSuite) TestGetMetadataBySHA256(c *gc.C) {
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

	p, err := NewService(s.state).GetMetadataBySHA256(context.Background(), sha)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(p, gc.DeepEquals, metadata)
}

func (s *serviceSuite) TestGetMetadataBySHA256Invalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sha256 := sha256.New()
	sha256.Write([]byte(uuid.MustNewUUID().String()))
	sha := hex.EncodeToString(sha256.Sum(nil))

	illegalSha := "!" + sha[1:]

	_, err := NewService(s.state).GetMetadataBySHA256(context.Background(), illegalSha)
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrInvalidHash)
}

func (s *serviceSuite) TestGetMetadataBySHA256TooShort(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sha256 := sha256.New()
	sha256.Write([]byte(uuid.MustNewUUID().String()))
	sha := hex.EncodeToString(sha256.Sum(nil))

	_, err := NewService(s.state).GetMetadataBySHA256(context.Background(), sha+"a")
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrInvalidHashLength)
}

func (s *serviceSuite) TestGetMetadataBySHA256TooLong(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetMetadataBySHA256(context.Background(), "beef")
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrInvalidHashLength)
}

func (s *serviceSuite) TestGetMetadataBySHA256Prefix(c *gc.C) {
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

	p, err := NewService(s.state).GetMetadataBySHA256Prefix(context.Background(), shaPrefix)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(p, gc.DeepEquals, metadata)
}

func (s *serviceSuite) TestGetMetadataBySHA256PrefixTooShort(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetMetadataBySHA256Prefix(context.Background(), "beef")
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrHashPrefixTooShort)
}

func (s *serviceSuite) TestGetMetadataBySHA256PrefixTooLong(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetMetadataBySHA256Prefix(context.Background(), "deadbeef1")
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrInvalidHashPrefix)
}

func (s *serviceSuite) TestGetMetadataBySHA256PrefixInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetMetadataBySHA256Prefix(context.Background(), "abcdefg")
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrInvalidHashPrefix)
}

func (s *serviceSuite) TestListMetadata(c *gc.C) {
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

	p, err := NewService(s.state).ListMetadata(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.DeepEquals, []objectstore.Metadata{{
		Path:   metadata.Path,
		Size:   metadata.Size,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
	}})
}

func (s *serviceSuite) TestPutMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := uuid.MustNewUUID().String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: uuid.MustNewUUID().String(),
		SHA384: uuid.MustNewUUID().String(),
		Size:   666,
	}

	uuid := objectstoretesting.GenObjectStoreUUID(c)
	s.state.EXPECT().PutMetadata(gomock.Any(), gomock.AssignableToTypeOf(objectstore.Metadata{})).DoAndReturn(func(ctx context.Context, data objectstore.Metadata) (objectstore.UUID, error) {
		c.Check(data.Path, gc.Equals, metadata.Path)
		c.Check(data.Size, gc.Equals, metadata.Size)
		c.Check(data.SHA256, gc.Equals, metadata.SHA256)
		c.Check(data.SHA384, gc.Equals, metadata.SHA384)
		return uuid, nil
	})

	result, err := NewService(s.state).PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, uuid)
}

func (s *serviceSuite) TestPutMetadataMissingSHA384(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := uuid.MustNewUUID().String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA256: uuid.MustNewUUID().String(),
		Size:   666,
	}

	_, err := NewService(s.state).PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrMissingHash)
}

func (s *serviceSuite) TestPutMetadataMissingSHA256(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := uuid.MustNewUUID().String()
	metadata := objectstore.Metadata{
		Path:   path,
		SHA384: uuid.MustNewUUID().String(),
		Size:   666,
	}

	_, err := NewService(s.state).PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrMissingHash)
}

func (s *serviceSuite) TestRemoveMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := uuid.MustNewUUID().String()

	s.state.EXPECT().RemoveMetadata(gomock.Any(), key).Return(nil)

	err := NewService(s.state).RemoveMetadata(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
}

// Test watch returns a watcher that watches the specified path.
func (s *serviceSuite) TestWatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	watcher := watchertest.NewMockStringsWatcher(nil)
	defer workertest.DirtyKill(c, watcher)

	table := "objectstore"
	stmt := "SELECT key FROM objectstore"
	s.state.EXPECT().InitialWatchStatement().Return(table, stmt)

	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any()).Return(watcher, nil)

	w, err := NewWatchableService(s.state, s.watcherFactory).Watch()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	return ctrl
}
