// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Storage holds the values for the hook context.
type Storage struct {
	Storage    map[names.StorageTag]jujuc.ContextStorageAttachment
	StorageTag names.StorageTag
	Added      map[string]params.StorageConstraints
}

// SetAttachment adds the attachment to the storage.
func (s *Storage) SetAttachment(attach jujuc.ContextStorageAttachment) {
	if attach == nil || attach == jujuc.ContextStorageAttachment(nil) {
		return
	}
	if s.Storage == nil {
		s.Storage = make(map[names.StorageTag]jujuc.ContextStorageAttachment)
	}
	s.Storage[attach.Tag()] = attach
}

// SetUnitStorage sets storage that should be added.
func (s *Storage) SetUnitStorage(name string, constraints params.StorageConstraints) {
	if s.Added == nil {
		s.Added = make(map[string]params.StorageConstraints)
	}
	s.Added[name] = constraints
}

// AddUnitStorage sets storage that should be added.
func (s *Storage) AddUnitStorage(all map[string]params.StorageConstraints) {
	if s.Added == nil {
		s.Added = make(map[string]params.StorageConstraints)
	}
	for k, v := range all {
		s.Added[k] = v
	}
}

// ContextStorage is a test double for jujuc.ContextStorage.
type ContextStorage struct {
	contextBase
	info *Storage
}

func (c *ContextStorage) setAttachment(name, location string, kind storage.StorageKind) {
	tag := names.NewStorageTag(name)
	attachment := &ContextStorageAttachment{
		info: &StorageAttachment{tag, kind, location},
	}
	attachment.stub = c.stub
	c.info.SetAttachment(attachment)
}

// Storage implements jujuc.ContextStorage.
func (c *ContextStorage) Storage(tag names.StorageTag) (jujuc.ContextStorageAttachment, bool) {
	c.stub.AddCall("Storage")
	c.stub.NextErr()

	storage, ok := c.info.Storage[tag]
	return storage, ok
}

// HookStorage implements jujuc.ContextStorage.
func (c *ContextStorage) HookStorage() (jujuc.ContextStorageAttachment, bool) {
	c.stub.AddCall("HookStorage")
	c.stub.NextErr()

	return c.Storage(c.info.StorageTag)
}

// AddUnitStorage implements jujuc.ContextStorage.
func (c *ContextStorage) AddUnitStorage(all map[string]params.StorageConstraints) {
	c.stub.AddCall("AddUnitStorage", all)
	c.stub.NextErr()

	c.info.AddUnitStorage(all)
}

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
