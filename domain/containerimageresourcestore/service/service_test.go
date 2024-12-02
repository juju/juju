// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	resourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/domain/application/resource"
	"github.com/juju/juju/domain/containerimageresourcestore"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/errors"
)

type containerImageResourceStoreSuite struct {
	testing.IsolationSuite
	containerImageResourceState *MockState
	imageMetadata               docker.DockerImageDetails
	file                        io.ReadCloser
}

var _ = gc.Suite(&containerImageResourceStoreSuite{})

func (s *containerImageResourceStoreSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.containerImageResourceState = NewMockState(ctrl)

	return ctrl
}

func (s *containerImageResourceStoreSuite) SetUpTest(c *gc.C) {
	s.imageMetadata = docker.DockerImageDetails{
		RegistryPath: "url@sha256:abc123",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "testuser",
				Password: "hunter2",
			},
		},
	}
	data, err := json.Marshal(s.imageMetadata)
	c.Assert(err, jc.ErrorIsNil)
	s.file = io.NopCloser(bytes.NewReader(data))
}

func (s *containerImageResourceStoreSuite) TestContainerImageResourceStorePut(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState)

	storageKey := resourcetesting.GenResourceUUID(c).String()
	expectedUUID := resource.ResourceStorageUUID("expected-uuid")
	s.containerImageResourceState.EXPECT().PutContainerImageMetadata(
		gomock.Any(),
		storageKey,
		s.imageMetadata.RegistryPath,
		s.imageMetadata.Username,
		s.imageMetadata.Password,
	).Return(expectedUUID, nil)

	storageUUID, err := store.Put(
		context.Background(),
		storageKey,
		s.file,
		0,
		charmresource.Fingerprint{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageUUID, gc.Equals, expectedUUID)
}

func (s *containerImageResourceStoreSuite) TestContainerImageResourceStorePutEmptyReader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState)

	storageKey := resourcetesting.GenResourceUUID(c).String()

	_, err := store.Put(
		context.Background(),
		storageKey,
		bytes.NewReader([]byte{}),
		0,
		charmresource.Fingerprint{},
	)
	c.Assert(err, gc.ErrorMatches, ".* zero bytes read")
}

func (s *containerImageResourceStoreSuite) TestContainerImageResourceStorePutError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState)

	storageKey := resourcetesting.GenResourceUUID(c).String()
	kaboom := errors.Errorf("kaboom")
	s.containerImageResourceState.EXPECT().PutContainerImageMetadata(
		gomock.Any(),
		storageKey,
		s.imageMetadata.RegistryPath,
		s.imageMetadata.Username,
		s.imageMetadata.Password,
	).Return("", kaboom)

	_, err := store.Put(
		context.Background(),
		storageKey,
		s.file,
		0,
		charmresource.Fingerprint{},
	)
	c.Assert(err, jc.ErrorIs, kaboom)
}

func (s *containerImageResourceStoreSuite) TestFileResourceStoreGet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState)

	storageKey := resourcetesting.GenResourceUUID(c).String()
	s.containerImageResourceState.EXPECT().GetContainerImageMetadata(
		gomock.Any(),
		storageKey,
	).Return(containerimageresourcestore.ContainerImageMetadata{
		StorageKey:   storageKey,
		RegistryPath: s.imageMetadata.RegistryPath,
		Username:     s.imageMetadata.Username,
		Password:     s.imageMetadata.Password,
	}, nil)

	r, _, err := store.Get(
		context.Background(),
		storageKey,
	)
	c.Assert(err, jc.ErrorIsNil)
	expectedReaderContents := new(bytes.Buffer)
	_, err = expectedReaderContents.ReadFrom(s.file)
	c.Assert(err, jc.ErrorIsNil)
	readerContents := new(bytes.Buffer)
	_, err = readerContents.ReadFrom(r)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(expectedReaderContents.String(), gc.Equals, readerContents.String())
}

func (s *containerImageResourceStoreSuite) TestFileResourceStoreGetError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState)

	storageKey := resourcetesting.GenResourceUUID(c).String()
	kaboom := errors.Errorf("kaboom")
	s.containerImageResourceState.EXPECT().GetContainerImageMetadata(
		gomock.Any(),
		storageKey,
	).Return(containerimageresourcestore.ContainerImageMetadata{}, kaboom)

	_, _, err := store.Get(
		context.Background(),
		storageKey,
	)
	c.Assert(err, jc.ErrorIs, kaboom)
}

func (s *containerImageResourceStoreSuite) TestFileResourceStoreRemove(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState)

	storageKey := resourcetesting.GenResourceUUID(c).String()
	s.containerImageResourceState.EXPECT().RemoveContainerImageMetadata(
		gomock.Any(),
		storageKey,
	)

	err := store.Remove(
		context.Background(),
		storageKey,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *containerImageResourceStoreSuite) TestFileResourceStoreRemoveError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState)

	kaboom := errors.Errorf("kaboom")
	storageKey := resourcetesting.GenResourceUUID(c).String()
	s.containerImageResourceState.EXPECT().RemoveContainerImageMetadata(
		gomock.Any(),
		storageKey,
	).Return(kaboom)

	err := store.Remove(
		context.Background(),
		storageKey,
	)
	c.Assert(err, jc.ErrorIs, kaboom)
}
