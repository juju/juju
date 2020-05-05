// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"github.com/juju/juju/storage"
	"github.com/juju/names/v4"
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
	c.stub.NextErr()

	return c.info.Tag
}

// Kind implements jujuc.StorageAttachement.
func (c *ContextStorageAttachment) Kind() storage.StorageKind {
	c.stub.AddCall("Kind")
	c.stub.NextErr()

	return c.info.Kind
}

// Location implements jujuc.StorageAttachement.
func (c *ContextStorageAttachment) Location() string {
	c.stub.AddCall("Location")
	c.stub.NextErr()

	return c.info.Location
}
