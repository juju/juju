// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	"github.com/juju/names"
)

// contextStorage is an implementation of jujuc.ContextStorage.
type contextStorage struct {
	tag      names.StorageTag
	kind     storage.StorageKind
	location string
}

// newContextStorage creates a new context for the given storage
// attachment.
func newContextStorage(a *params.StorageAttachment) (*contextStorage, error) {
	tag, err := names.ParseStorageTag(a.StorageTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	kind := storage.StorageKind(a.Kind)
	return &contextStorage{
		tag:      tag,
		kind:     kind,
		location: a.Location,
	}, nil
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
