// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/charm/downloader"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
)

var _ = gc.Suite(&storageTestSuite{})

type storageTestSuite struct {
	testing.IsolationSuite

	stateBackend   *MockStateBackend
	uploadedCharm  *MockUploadedCharm
	storageBackend *MockStorage
	storage        *CharmStorage
	uuid           uuid.UUID
}

func (s *storageTestSuite) TestPrepareToStoreNotYetUploadedCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := "ch:ubuntu-lite"

	s.stateBackend.EXPECT().PrepareCharmUpload(curl).Return(s.uploadedCharm, nil)
	s.uploadedCharm.EXPECT().IsUploaded().Return(false)

	err := s.storage.PrepareToStoreCharm(curl)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageTestSuite) TestPrepareToStoreAlreadyUploadedCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := "ch:ubuntu-lite"

	s.stateBackend.EXPECT().PrepareCharmUpload(curl).Return(s.uploadedCharm, nil)
	s.uploadedCharm.EXPECT().IsUploaded().Return(true)

	err := s.storage.PrepareToStoreCharm(curl)

	expErr := downloader.NewCharmAlreadyStoredError(curl)
	c.Assert(err, gc.Equals, expErr)
}

func (s *storageTestSuite) TestStoreBlobFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := "ch:ubuntu-lite"
	expStoreCharmPath := fmt.Sprintf("charms/%s-%s", curl, s.uuid)
	dlCharm := downloader.DownloadedCharm{
		CharmData: strings.NewReader("the-blob"),
		Size:      7337,
	}

	s.storageBackend.EXPECT().Put(gomock.Any(), expStoreCharmPath, gomock.AssignableToTypeOf(dlCharm.CharmData), int64(7337)).Return(errors.New("failed"))

	err := s.storage.Store(context.Background(), curl, dlCharm)
	c.Assert(err, gc.ErrorMatches, "cannot add charm to storage.*")
}

func (s *storageTestSuite) TestStoreBlobAlreadyStored(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := "ch:ubuntu-lite"
	expStoreCharmPath := fmt.Sprintf("charms/%s-%s", curl, s.uuid)
	dlCharm := downloader.DownloadedCharm{
		CharmData:    strings.NewReader("the-blob"),
		Size:         7337,
		SHA256:       "d357",
		CharmVersion: "the-version",
	}

	s.storageBackend.EXPECT().Put(gomock.Any(), expStoreCharmPath, gomock.AssignableToTypeOf(dlCharm.CharmData), int64(7337)).Return(nil)
	s.stateBackend.EXPECT().UpdateUploadedCharm(state.CharmInfo{
		StoragePath: expStoreCharmPath,
		ID:          curl,
		SHA256:      "d357",
		Version:     "the-version",
	}).Return(nil, stateerrors.NewErrCharmAlreadyUploaded(curl))

	// As the blob is already uploaded (to another path), we need to remove
	// the duplicate we just uploaded from the store.
	s.storageBackend.EXPECT().Remove(gomock.Any(), expStoreCharmPath).Return(nil)

	err := s.storage.Store(context.Background(), curl, dlCharm)
	c.Assert(err, jc.ErrorIsNil) // charm already uploaded by someone; no error
}

func (s *storageTestSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.stateBackend = NewMockStateBackend(ctrl)
	s.uploadedCharm = NewMockUploadedCharm(ctrl)
	s.storageBackend = NewMockStorage(ctrl)

	var err error
	s.uuid, err = uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.storage = NewCharmStorage(CharmStorageConfig{
		Logger:       loggo.GetLogger("test"),
		StateBackend: s.stateBackend,
		ObjectStore:  s.storageBackend,
	})
	s.storage.uuidGenerator = func() (uuid.UUID, error) {
		return s.uuid, nil
	}

	return ctrl
}
