// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"io"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	coreresource "github.com/juju/juju/core/resource"
	resourcestore "github.com/juju/juju/core/resource/store"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
)

type fileResourceStoreSuite struct {
	testhelpers.IsolationSuite
	objectStore *MockObjectStore
	resource    coreresource.Resource
	file        io.ReadCloser
}

func TestFileResourceStoreSuite(t *testing.T) {
	tc.Run(t, &fileResourceStoreSuite{})
}

func (s *fileResourceStoreSuite) SetUpTest(c *tc.C) {
	data := "spamspamspam"
	size := int64(len(data))
	sha384hash := sha512.New384()
	sha384hash.Write([]byte(data))
	fp := fmt.Sprintf("%x", sha384hash.Sum(nil))
	fingerprint, err := charmresource.ParseFingerprint(fp)
	c.Assert(err, tc.ErrorIsNil)
	s.resource = coreresource.Resource{
		UUID: coreresource.GenUUID(c),
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
		c.Context(),
		s.resource.UUID.String(),
		s.file,
		s.resource.Size,
		s.resource.Fingerprint.String(),
	).Return(expectedStorageUUID, nil)

	fingerprint := resourcestore.NewFingerprint(s.resource.Fingerprint.Fingerprint)
	size := s.resource.Size
	storageUUID, returnedSize, returnedFingerprint, err := store.Put(
		c.Context(),
		s.resource.UUID.String(),
		s.file,
		size,
		fingerprint,
	)
	c.Assert(err, tc.ErrorIsNil)

	id, err := storageUUID.ObjectStoreUUID()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(id, tc.Equals, expectedStorageUUID)
	c.Check(returnedSize, tc.Equals, size)
	c.Check(returnedFingerprint, tc.DeepEquals, fingerprint)
}

func (s *fileResourceStoreSuite) TestFileResourceStorePutBadStorageKey(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}
	_, _, _, err := store.Put(
		c.Context(),
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
		c.Context(),
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
		c.Context(),
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
		c.Context(),
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

	s.objectStore.EXPECT().Get(gomock.Any(), s.resource.UUID.String()).Return(s.file, s.resource.Size, nil)

	reader, size, err := store.Get(c.Context(), s.resource.UUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reader, tc.Equals, s.file)
	c.Check(size, tc.Equals, s.resource.Size)
}

func (s *fileResourceStoreSuite) TestFileResourceStoreGetBadStorageKey(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}
	_, _, err := store.Get(
		c.Context(),
		"",
	)
	c.Assert(err, tc.ErrorMatches, "storage key empty")
}

func (s *fileResourceStoreSuite) TestFileResourceStoreRemove(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}

	s.objectStore.EXPECT().Remove(gomock.Any(), s.resource.UUID.String()).Return(nil)

	err := store.Remove(c.Context(), s.resource.UUID.String())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *fileResourceStoreSuite) TestFileResourceStoreRemoveBadStorageKey(c *tc.C) {
	defer s.setupMocks(c).Finish()
	store := fileResourceStore{s.objectStore}
	err := store.Remove(
		c.Context(),
		"",
	)
	c.Assert(err, tc.ErrorMatches, "storage key empty")
}
