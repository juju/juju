// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/names"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Storage holds the values for the hook context.
type Storage struct {
	Storage    map[names.StorageTag]*ContextStorageAttachment
	StorageTag names.StorageTag
}

func (s *Storage) setAttachment(attach *ContextStorageAttachment) {
	if attach == nil {
		return
	}
	if s.Storage == nil {
		s.Storage = make(map[names.StorageTag]*ContextStorageAttachment)
	}
	s.Storage[attach.Info.Tag] = attach
}

// ContextStorage is a test double for jujuc.ContextStorage.
type ContextStorage struct {
	Stub *testing.Stub
	Info *Storage
}

func (c *ContextStorage) init() {
	if c.Stub == nil {
		c.Stub = &testing.Stub{}
	}
	if c.Info == nil {
		c.Info = &Storage{}
	}
}

func (c *ContextStorage) setAttachment(name, location string, kind storage.StorageKind) {
	c.init()
	tag := names.NewStorageTag(name)
	attachment := &ContextStorageAttachment{
		Stub: c.Stub,
		Info: &StorageAttachment{tag, kind, location},
	}
	c.Info.setAttachment(attachment)
}

// Storage implements jujuc.ContextStorage.
func (cs *ContextStorage) Storage(tag names.StorageTag) (jujuc.ContextStorageAttachment, bool) {
	cs.Stub.AddCall("Storage")
	cs.Stub.NextErr()
	cs.init()
	storage, ok := cs.Info.Storage[tag]
	return storage, ok
}

// HookStorage implements jujuc.ContextStorage.
func (cs *ContextStorage) HookStorage() (jujuc.ContextStorageAttachment, bool) {
	cs.Stub.AddCall("HookStorage")
	cs.Stub.NextErr()
	cs.init()
	return cs.Storage(cs.Info.StorageTag)
}

// AddUnitStorage implements jujuc.ContextStorage.
func (cs *ContextStorage) AddUnitStorage(all map[string]params.StorageConstraints) {
	cs.Stub.AddCall("AddUnitStorage", all)
	cs.Stub.NextErr()
	cs.init()
}

// StorageAttachment holds the data for the test double.
type StorageAttachment struct {
	Tag      names.StorageTag
	Kind     storage.StorageKind
	Location string
}

// ContextStorageAttachment is a test double for jujuc.ContextStorageAttachment.
type ContextStorageAttachment struct {
	Stub *testing.Stub
	Info *StorageAttachment
}

func (csa *ContextStorageAttachment) init() {
	if csa.Stub == nil {
		csa.Stub = &testing.Stub{}
	}
	if csa.Info == nil {
		csa.Info = &StorageAttachment{}
	}
}

// Tag implements jujuc.StorageAttachement.
func (csa *ContextStorageAttachment) Tag() names.StorageTag {
	csa.Stub.AddCall("Tag")
	csa.Stub.NextErr()
	csa.init()
	return csa.Info.Tag
}

// Kind implements jujuc.StorageAttachement.
func (csa *ContextStorageAttachment) Kind() storage.StorageKind {
	csa.Stub.AddCall("Kind")
	csa.Stub.NextErr()
	csa.init()
	return csa.Info.Kind
}

// Location implements jujuc.StorageAttachement.
func (csa *ContextStorageAttachment) Location() string {
	csa.Stub.AddCall("Location")
	csa.Stub.NextErr()
	csa.init()
	return csa.Info.Location
}
