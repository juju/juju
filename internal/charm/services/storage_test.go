// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	charm "github.com/juju/juju/core/charm"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/charm/downloader"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

var _ = gc.Suite(&storageTestSuite{})

type storageTestSuite struct {
	testing.IsolationSuite

	storageBackend     *MockStorage
	storage            *CharmStorage
	uuid               uuid.UUID
	applicationService *MockApplicationService
}

func (s *storageTestSuite) TestPrepareToStoreNotYetUploadedCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := "ch:ubuntu-lite"

	s.applicationService.EXPECT().GetCharmID(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, args applicationcharm.GetCharmArgs) (charm.ID, error) {
			c.Check(args.Name, gc.Equals, "ubuntu-lite")
			c.Check(args.Source, gc.Equals, applicationcharm.CharmHubSource)
			return "charm0", nil
		})
	s.applicationService.EXPECT().GetCharm(gomock.Any(), charm.ID("charm0")).Return(nil, applicationcharm.CharmLocator{}, false, nil)

	err := s.storage.PrepareToStoreCharm(curl)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageTestSuite) TestPrepareToStoreAlreadyUploadedCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := "ch:ubuntu-lite"

	s.applicationService.EXPECT().GetCharmID(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, args applicationcharm.GetCharmArgs) (charm.ID, error) {
			c.Check(args.Name, gc.Equals, "ubuntu-lite")
			c.Check(args.Source, gc.Equals, applicationcharm.CharmHubSource)
			return "charm0", nil
		})
	s.applicationService.EXPECT().GetCharm(gomock.Any(), charm.ID("charm0")).Return(nil, applicationcharm.CharmLocator{}, true, nil)

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

	s.storageBackend.EXPECT().Put(gomock.Any(), expStoreCharmPath, gomock.AssignableToTypeOf(dlCharm.CharmData), int64(7337)).Return("", errors.New("failed"))

	_, err := s.storage.Store(context.Background(), curl, dlCharm)
	c.Assert(err, gc.ErrorMatches, "cannot add charm to storage.*")
}

func (s *storageTestSuite) TestStoreBlobAlreadyStored(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Uploading the same blob to the _objectstore_ twice should still
	// succeed.

	curl := "ch:ubuntu-lite"
	expStoreCharmPath := fmt.Sprintf("charms/%s-%s", curl, s.uuid)
	dlCharm := downloader.DownloadedCharm{
		CharmData:    strings.NewReader("the-blob"),
		Size:         7337,
		SHA256:       "d357",
		CharmVersion: "the-version",
	}

	s.storageBackend.EXPECT().Put(gomock.Any(), expStoreCharmPath, gomock.AssignableToTypeOf(dlCharm.CharmData), int64(7337)).Return("", objectstoreerrors.ErrPathAlreadyExistsDifferentHash)

	_, err := s.storage.Store(context.Background(), curl, dlCharm)
	c.Assert(err, jc.ErrorIsNil) // charm already uploaded by someone; no error
}

func (s *storageTestSuite) TestStoreCharmAlreadyStored(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Uploading the same charm to the state layer twice should still
	// succeed.

	curl := "ch:ubuntu-lite"
	expStoreCharmPath := fmt.Sprintf("charms/%s-%s", curl, s.uuid)
	dlCharm := downloader.DownloadedCharm{
		CharmData:    strings.NewReader("the-blob"),
		Size:         7337,
		SHA256:       "d357",
		CharmVersion: "the-version",
	}

	s.storageBackend.EXPECT().Put(gomock.Any(), expStoreCharmPath, gomock.AssignableToTypeOf(dlCharm.CharmData), int64(7337)).Return("", nil)

	_, err := s.storage.Store(context.Background(), curl, dlCharm)
	c.Assert(err, jc.ErrorIsNil) // charm already uploaded by someone; no error
}

func (s *storageTestSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.storageBackend = NewMockStorage(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)

	var err error
	s.uuid, err = uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.storage = NewCharmStorage(CharmStorageConfig{
		Logger:             loggertesting.WrapCheckLog(c),
		ObjectStore:        s.storageBackend,
		ApplicationService: s.applicationService,
	})
	s.storage.uuidGenerator = func() (uuid.UUID, error) {
		return s.uuid, nil
	}

	return ctrl
}
