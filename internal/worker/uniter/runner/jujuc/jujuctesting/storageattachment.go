// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/internal/storage"
)

// StorageAttachment holds the data for the test double.
type StorageAttachment struct {
	Tag      names.StorageTag
	Kind     storage.StorageKind
	Location string
}

// ContextStorageAttachment is a test double for jujuc.ContextStorageAttachment.
type ContextStorageAttachment struct {
	contextBase
	info *StorageAttachment
}

// Tag implements jujuc.StorageAttachement.
func (c *ContextStorageAttachment) Tag() names.StorageTag {
	c.stub.AddCall("Tag")
	_ = c.stub.NextErr()

	return c.info.Tag
}

// Kind implements jujuc.StorageAttachement.
func (c *ContextStorageAttachment) Kind() storage.StorageKind {
	c.stub.AddCall("Kind")
	_ = c.stub.NextErr()

	return c.info.Kind
}

// Location implements jujuc.StorageAttachement.
func (c *ContextStorageAttachment) Location() string {
	c.stub.AddCall("Location")
	_ = c.stub.NextErr()

	return c.info.Location
}
