// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"fmt"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/charm/downloader"
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
	uuid           utils.UUID
}

func (s *storageTestSuite) TestPrepareToStoreNotYetUploadedCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("ch:ubuntu-lite")

	s.stateBackend.EXPECT().PrepareCharmUpload(curl).Return(s.uploadedCharm, nil)
	s.uploadedCharm.EXPECT().IsUploaded().Return(false)

	err := s.storage.PrepareToStoreCharm(curl)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageTestSuite) TestPrepareToStoreAlreadyUploadedCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("ch:ubuntu-lite")
	dlOrigin := corecharm.Origin{
		Source: corecharm.CharmHub,
		Type:   "charm",
		ID:     "42",
		Hash:   "092134u093ruj23",
	}

	s.stateBackend.EXPECT().PrepareCharmUpload(curl).Return(s.uploadedCharm, nil)
	s.uploadedCharm.EXPECT().IsUploaded().Return(true)
	s.stateBackend.EXPECT().UploadedCharmOrigin(curl).Return(dlOrigin, nil)

	err := s.storage.PrepareToStoreCharm(curl)

	expErr := downloader.NewCharmAlreadyStoredError(curl.String(), dlOrigin)
	c.Assert(err, gc.Equals, expErr)
}

func (s *storageTestSuite) TestPrepareToStoreAlreadyUploadedCharmOriginNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("ch:ubuntu-lite")

	s.stateBackend.EXPECT().PrepareCharmUpload(curl).Return(s.uploadedCharm, nil)
	s.uploadedCharm.EXPECT().IsUploaded().Return(true)

	s.stateBackend.EXPECT().UploadedCharmOrigin(curl).Return(corecharm.Origin{}, errors.NotFoundf("charm origin"))

	err := s.storage.PrepareToStoreCharm(curl)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageTestSuite) TestStoreBlobFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("ch:ubuntu-lite")
	expStoreCharmPath := fmt.Sprintf("charms/%s-%s", curl.String(), s.uuid)
	dlCharm := downloader.DownloadedCharm{
		CharmData: strings.NewReader("the-blob"),
		Size:      7337,
	}

	s.stateBackend.EXPECT().ModelUUID().Return("the-model-uuid")
	s.storageBackend.EXPECT().Put(expStoreCharmPath, gomock.AssignableToTypeOf(dlCharm.CharmData), int64(7337)).Return(errors.New("failed"))

	err := s.storage.Store(curl, dlCharm)
	c.Assert(err, gc.ErrorMatches, "cannot add charm to storage.*")
}

func (s *storageTestSuite) TestStoreBlobAlreadyStored(c *gc.C) {
	defer s.setupMocks(c).Finish()

	mac, err := macaroon.New(nil, []byte("id"), "", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	macaroons := macaroon.Slice{mac}

	curl := charm.MustParseURL("ch:ubuntu-lite")
	expStoreCharmPath := fmt.Sprintf("charms/%s-%s", curl.String(), s.uuid)
	dlCharm := downloader.DownloadedCharm{
		CharmData:    strings.NewReader("the-blob"),
		Size:         7337,
		SHA256:       "d357",
		CharmVersion: "the-version",
		Macaroons:    macaroons,
	}

	s.stateBackend.EXPECT().ModelUUID().Return("the-model-uuid")
	s.storageBackend.EXPECT().Put(expStoreCharmPath, gomock.AssignableToTypeOf(dlCharm.CharmData), int64(7337)).Return(nil)
	s.stateBackend.EXPECT().UpdateUploadedCharm(state.CharmInfo{
		StoragePath: expStoreCharmPath,
		ID:          curl,
		SHA256:      "d357",
		Version:     "the-version",
		Macaroon:    macaroons,
	}).Return(nil, stateerrors.NewErrCharmAlreadyUploaded(curl))

	// As the blob is already uploaded (to another path), we need to remove
	// the duplicate we just uploaded from the store.
	s.storageBackend.EXPECT().Remove(expStoreCharmPath).Return(nil)

	err = s.storage.Store(curl, dlCharm)
	c.Assert(err, jc.ErrorIsNil) // charm already uploaded by someone; no error
}

func (s *storageTestSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.stateBackend = NewMockStateBackend(ctrl)
	s.uploadedCharm = NewMockUploadedCharm(ctrl)
	s.storageBackend = NewMockStorage(ctrl)

	var err error
	s.uuid, err = utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.storage = NewCharmStorage(CharmStorageConfig{
		Logger:       loggo.GetLogger("test"),
		StateBackend: s.stateBackend,
		StorageFactory: func(_ string) Storage {
			return s.storageBackend
		},
	})
	s.storage.uuidGenerator = func() (utils.UUID, error) {
		return s.uuid, nil
	}

	return ctrl
}
