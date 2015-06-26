// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/names"

	"github.com/juju/juju/storage"
)

// contextStorage is an implementation of jujuc.ContextStorageAttachment.
type contextStorage struct {
	tag      names.StorageTag
	kind     storage.StorageKind
	location string
}

func (ctx *contextStorage) Tag() names.StorageTag {
	return ctx.tag
}

func (ctx *contextStorage) Kind() storage.StorageKind {
	return ctx.kind
}

func (ctx *contextStorage) Location() string {
	return ctx.location
}
