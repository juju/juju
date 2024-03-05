// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/internal/storage"
)

// contextStorage is an implementation of hooks.ContextStorageAttachment.
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
