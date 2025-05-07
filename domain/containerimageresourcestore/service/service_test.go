// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v2"

	resourcestore "github.com/juju/juju/core/resource/store"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/domain/containerimageresourcestore"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type containerImageResourceStoreSuite struct {
	testing.IsolationSuite
	containerImageResourceState *MockState
	imageMetadata               docker.DockerImageDetails
	jsonBlob                    io.ReadCloser
	yamlBlob                    io.ReadCloser
	size                        int64
	fingerprint                 resourcestore.Fingerprint
}

var _ = tc.Suite(&containerImageResourceStoreSuite{})

func (s *containerImageResourceStoreSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.containerImageResourceState = NewMockState(ctrl)

	return ctrl
}

func (s *containerImageResourceStoreSuite) SetUpTest(c *tc.C) {
	s.imageMetadata = docker.DockerImageDetails{
		RegistryPath: "url@sha256:abc123",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "testuser",
				Password: "hunter2",
			},
		},
	}
	jsonData, err := json.Marshal(s.imageMetadata)
	c.Assert(err, tc.ErrorIsNil)
	s.jsonBlob = io.NopCloser(bytes.NewReader(jsonData))

	reader := bytes.NewReader(jsonData)
	s.size = int64(reader.Len())
	fingerprint, err := charmresource.GenerateFingerprint(reader)
	c.Assert(err, tc.ErrorIsNil)
	s.fingerprint = resourcestore.NewFingerprint(fingerprint.Fingerprint)

	yamlData, err := yaml.Marshal(s.imageMetadata)
	c.Assert(err, tc.ErrorIsNil)
	s.yamlBlob = io.NopCloser(bytes.NewReader(yamlData))
}

func (s *containerImageResourceStoreSuite) TestContainerImageResourceStorePutJson(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState, loggertesting.WrapCheckLog(c))

	storageKey := resourcetesting.GenResourceUUID(c).String()
	expectedUUID, err := resourcestore.NewContainerImageMetadataResourceID("expected-uuid")
	c.Assert(err, tc.ErrorIsNil)
	s.containerImageResourceState.EXPECT().PutContainerImageMetadata(
		gomock.Any(),
		storageKey,
		s.imageMetadata.RegistryPath,
		s.imageMetadata.Username,
		s.imageMetadata.Password,
	).Return(expectedUUID, nil)

	storageUUID, size, fingerprint, err := store.Put(
		context.Background(),
		storageKey,
		s.jsonBlob,
		0,
		resourcestore.Fingerprint{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageUUID, tc.Equals, expectedUUID)
	c.Assert(size, tc.Equals, s.size)
	c.Assert(fingerprint, tc.DeepEquals, s.fingerprint)
}

func (s *containerImageResourceStoreSuite) TestContainerImageResourceStorePutYaml(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState, loggertesting.WrapCheckLog(c))

	storageKey := resourcetesting.GenResourceUUID(c).String()
	expectedUUID, err := resourcestore.NewContainerImageMetadataResourceID("expected-uuid")
	c.Assert(err, tc.ErrorIsNil)
	s.containerImageResourceState.EXPECT().PutContainerImageMetadata(
		gomock.Any(),
		storageKey,
		s.imageMetadata.RegistryPath,
		s.imageMetadata.Username,
		s.imageMetadata.Password,
	).Return(expectedUUID, nil)

	storageUUID, size, fingerprint, err := store.Put(
		context.Background(),
		storageKey,
		s.yamlBlob,
		0,
		resourcestore.Fingerprint{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageUUID, tc.Equals, expectedUUID)
	c.Assert(size, tc.Equals, s.size)
	c.Assert(fingerprint, tc.DeepEquals, s.fingerprint)
}

func (s *containerImageResourceStoreSuite) TestContainerImageResourceStorePutEmptyReader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState, loggertesting.WrapCheckLog(c))

	storageKey := resourcetesting.GenResourceUUID(c).String()

	_, _, _, err := store.Put(
		context.Background(),
		storageKey,
		bytes.NewReader([]byte{}),
		0,
		resourcestore.Fingerprint{},
	)
	c.Assert(err, tc.ErrorMatches, ".* zero bytes read")
}

func (s *containerImageResourceStoreSuite) TestContainerImageResourceStorePutError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState, loggertesting.WrapCheckLog(c))

	storageKey := resourcetesting.GenResourceUUID(c).String()
	kaboom := errors.Errorf("kaboom")
	s.containerImageResourceState.EXPECT().PutContainerImageMetadata(
		gomock.Any(),
		storageKey,
		s.imageMetadata.RegistryPath,
		s.imageMetadata.Username,
		s.imageMetadata.Password,
	).Return(resourcestore.ID{}, kaboom)

	_, _, _, err := store.Put(
		context.Background(),
		storageKey,
		s.jsonBlob,
		0,
		resourcestore.Fingerprint{},
	)
	c.Assert(err, tc.ErrorIs, kaboom)
}

func (s *containerImageResourceStoreSuite) TestFileResourceStoreGet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState, loggertesting.WrapCheckLog(c))

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

	r, size, err := store.Get(
		context.Background(),
		storageKey,
	)
	c.Assert(err, tc.ErrorIsNil)
	expectedReaderContents := new(bytes.Buffer)
	expectedSize, err := expectedReaderContents.ReadFrom(s.jsonBlob)
	c.Assert(err, tc.ErrorIsNil)
	readerContents := new(bytes.Buffer)
	_, err = readerContents.ReadFrom(r)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(expectedReaderContents.String(), tc.Equals, readerContents.String())
	c.Assert(size, tc.Equals, expectedSize)
}

func (s *containerImageResourceStoreSuite) TestFileResourceStoreGetError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState, loggertesting.WrapCheckLog(c))

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
	c.Assert(err, tc.ErrorIs, kaboom)
}

func (s *containerImageResourceStoreSuite) TestFileResourceStoreRemove(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState, loggertesting.WrapCheckLog(c))

	storageKey := resourcetesting.GenResourceUUID(c).String()
	s.containerImageResourceState.EXPECT().RemoveContainerImageMetadata(
		gomock.Any(),
		storageKey,
	)

	err := store.Remove(
		context.Background(),
		storageKey,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *containerImageResourceStoreSuite) TestFileResourceStoreRemoveError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := NewService(s.containerImageResourceState, loggertesting.WrapCheckLog(c))

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
	c.Assert(err, tc.ErrorIs, kaboom)
}
