// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	coreunit "github.com/juju/juju/core/unit"
	domainapplicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
)

// attachmentSuite is a test suite for asserting the behaviour of storage
// attachment related methods on [State].
type attachmentSuite struct {
	baseSuite
}

// attachmentUUIDSuite is a test suite for asserting the behaviour of
// [State.GetStorageAttachmentUUIDForStorageInstanceAndUnit].
//
// NOTE (tlm): This was made into its own suite to keep the test name length
// under control.
type attachmentUUIDSuite struct {
	baseSuite
}

// TestAttachmentSuite runs all of the tests contained within [attachmentSuite].
func TestAttachmentSuite(t *testing.T) {
	tc.Run(t, &attachmentSuite{})
}

// TestAttachmentUUIDSuite runs the tests contained in [attachmentUUIDSuite].
func TestAttachmentUUIDSuite(t *testing.T) {
	tc.Run(t, &attachmentUUIDSuite{})
}

// TestUUIDForNotFoundUnit asserts that when a unit does not exist
// [State.GetStorageAttachmentUUIDForStorageInstanceAndUnit] returns a
// [domainapplicationerrors.UnitNotFound] error.
func (s *attachmentUUIDSuite) TestUUIDForNotFoundUnit(c *tc.C) {
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	storageInstanceUUID, _ := s.newStorageInstanceForCharmWithPool(
		c, "kratos", poolUUID, "token-store",
	)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), storageInstanceUUID, unitUUID,
	)
	c.Check(err, tc.ErrorIs, domainapplicationerrors.UnitNotFound)
}

// TestUUIDForNotFoundStorageInstance asserts that when a storage
// instance does not exist
// [State.GetStorageAttachmentUUIDForStorageInstanceAndUnit] returns a
// [domainstorageerrors.StorageInstanceNotFound] error.
func (s *attachmentUUIDSuite) TestUUIDForNotFoundStorageInstance(c *tc.C) {
	unitUUID := s.newUnit(c)
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), storageInstanceUUID, unitUUID,
	)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestUUIDForStorageInstanceAndUnit is a happy path test.
func (s *attachmentUUIDSuite) TestUUIDForStorageInstanceAndUnit(c *tc.C) {
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	storageInstanceUUID, _ := s.newStorageInstanceForCharmWithPool(
		c, "kratos", poolUUID, "token-store",
	)
	unitUUID := s.newUnit(c)
	storageAttachmentUUID := s.newStorageAttachment(
		c, storageInstanceUUID, unitUUID,
	)

	st := NewState(s.TxnRunnerFactory())
	gotUUID, err := st.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), storageInstanceUUID, unitUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, storageAttachmentUUID)
}

// TestGetStorageInstnaceAttachmentsNotFound asserts that when the storage
// instance does not exist in the model the caller gets back an error satisfying
// [domainstorageerrors.StorageInstanceNotFound].
func (s *attachmentSuite) TestGetStorageInstanceAttachmentsNotFound(c *tc.C) {
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageInstanceAttachments(c.Context(), storageInstanceUUID)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestGetStorageInstanceAttachmentsEmptyResult asserts that when a storage
// instance is not attached to any units an empty slice is returned.
func (s *attachmentSuite) TestGetStorageInstanceAttachmentsEmptyResult(c *tc.C) {
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	storageInstanceUUID, _ := s.newStorageInstanceForCharmWithPool(
		c, "kratos", poolUUID, "token-store",
	)

	st := NewState(s.TxnRunnerFactory())
	attachments, err := st.GetStorageInstanceAttachments(c.Context(), storageInstanceUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(attachments, tc.HasLen, 0)
}

func (s *attachmentSuite) TestGetStorageInstanceAttachments(c *tc.C) {
	unitUUID1 := s.newUnit(c)
	unitUUID2 := s.newUnit(c)
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	storageInstanceUUID, _ := s.newStorageInstanceForCharmWithPool(
		c, "kratos", poolUUID, "token-store",
	)
	storageAttachmentUUID1 := s.newStorageAttachment(c, storageInstanceUUID, unitUUID1)
	storageAttachmentUUID2 := s.newStorageAttachment(c, storageInstanceUUID, unitUUID2)

	st := NewState(s.TxnRunnerFactory())
	attachments, err := st.GetStorageInstanceAttachments(c.Context(), storageInstanceUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(
		attachments, tc.SameContents,
		[]domainstorageprovisioning.StorageAttachmentUUID{
			storageAttachmentUUID2,
			storageAttachmentUUID1,
		},
	)
}
