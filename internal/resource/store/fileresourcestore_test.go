// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"bytes"
	"context"
	"crypto/sha512"
	"fmt"
	"io"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	coreresource "github.com/juju/juju/core/resource"
	resourcestore "github.com/juju/juju/core/resource/store"
	resourcestesting "github.com/juju/juju/core/resource/testing"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

type fileResourceStoreSuite struct {
	testing.IsolationSuite
	objectStore *MockObjectStore
	resource    coreresource.Resource
	file        io.ReadCloser
}

var _ = tc.Suite(&fileResourceStoreSuite{})

func (s *fileResourceStoreSuite) SetUpTest(c *tc.C) {
	data := "spamspamspam"
	size := int64(len(data))
	sha384hash := sha512.New384()
	sha384hash.Write([]byte(data))
	fp := fmt.Sprintf("%x", sha384hash.Sum(nil))
	fingerprint, err := charmresource.ParseFingerprint(fp)
	c.Assert(err, jc.ErrorIsNil)
	s.resource = coreresource.Resource{
		UUID: resourcestesting.GenResourceUUID(c),
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "spam-resource",
				Type: charmresource.TypeFile,
			},
			Fingerprint: fingerprint,
			Size:        size,
		},
		ApplicationName: "fake-app-uuid",
	}
	s.file = io.NopCloser(bytes.NewBufferString(data))
}

func (s *fileResourceStoreSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.objectStore = NewMockObjectStore(ctrl)

	return ctrl
}

func (s *fileResourceStoreSuite) TestFileResourceStorePut(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}

	expectedStorageUUID := objectstoretesting.GenObjectStoreUUID(c)
	s.objectStore.EXPECT().PutAndCheckHash(
		context.Background(),
		s.resource.UUID.String(),
		s.file,
		s.resource.Size,
		s.resource.Fingerprint.String(),
	).Return(expectedStorageUUID, nil)

	fingerprint := resourcestore.NewFingerprint(s.resource.Fingerprint.Fingerprint)
	size := s.resource.Size
	storageUUID, returnedSize, returnedFingerprint, err := store.Put(
		context.Background(),
		s.resource.UUID.String(),
		s.file,
		size,
		fingerprint,
	)
	c.Assert(err, jc.ErrorIsNil)

	id, err := storageUUID.ObjectStoreUUID()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(id, tc.Equals, expectedStorageUUID)
	c.Check(returnedSize, tc.Equals, size)
	c.Check(returnedFingerprint, tc.DeepEquals, fingerprint)
}

func (s *fileResourceStoreSuite) TestFileResourceStorePutBadStorageKey(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}
	_, _, _, err := store.Put(
		context.Background(),
		"",
		s.file,
		s.resource.Size,
		resourcestore.NewFingerprint(s.resource.Fingerprint.Fingerprint),
	)
	c.Assert(err, tc.ErrorMatches, "storage key empty")
}

func (s *fileResourceStoreSuite) TestFileResourceStorePutNilReader(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}
	_, _, _, err := store.Put(
		context.Background(),
		s.resource.UUID.String(),
		nil,
		s.resource.Size,
		resourcestore.NewFingerprint(s.resource.Fingerprint.Fingerprint),
	)
	c.Assert(err, tc.ErrorMatches, "validating resource: reader is nil")
}

func (s *fileResourceStoreSuite) TestFileResourceStorePutBadFingerprint(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}
	_, _, _, err := store.Put(
		context.Background(),
		s.resource.UUID.String(),
		s.file,
		s.resource.Size,
		resourcestore.Fingerprint{},
	)
	c.Assert(err, tc.ErrorMatches, "validating resource fingerprint: .*")
}

func (s *fileResourceStoreSuite) TestFileResourceStorePutZeroSize(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}
	_, _, _, err := store.Put(
		context.Background(),
		s.resource.UUID.String(),
		s.file,
		0,
		resourcestore.Fingerprint{},
	)
	c.Assert(err, tc.ErrorMatches, "validating resource size: size is 0")
}

func (s *fileResourceStoreSuite) TestFileResourceStoreGet(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}

	s.objectStore.EXPECT().Get(context.Background(), s.resource.UUID.String()).Return(s.file, s.resource.Size, nil)

	reader, size, err := store.Get(context.Background(), s.resource.UUID.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reader, tc.Equals, s.file)
	c.Check(size, tc.Equals, s.resource.Size)
}

func (s *fileResourceStoreSuite) TestFileResourceStoreGetBadStorageKey(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}
	_, _, err := store.Get(
		context.Background(),
		"",
	)
	c.Assert(err, tc.ErrorMatches, "storage key empty")
}

func (s *fileResourceStoreSuite) TestFileResourceStoreRemove(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}

	s.objectStore.EXPECT().Remove(context.Background(), s.resource.UUID.String()).Return(nil)

	err := store.Remove(context.Background(), s.resource.UUID.String())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *fileResourceStoreSuite) TestFileResourceStoreRemoveBadStorageKey(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}
	err := store.Remove(
		context.Background(),
		"",
	)
	c.Assert(err, tc.ErrorMatches, "storage key empty")
}
