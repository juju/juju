// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
)

// newStorageIdValue returns a gnuflag.Value for convenient parsing of storage
// ids in ctx.
func newStorageIdValue(ctx Context, result *names.StorageTag) (*storageIdValue, error) {
	v := &storageIdValue{result: result, ctx: ctx}
	if s, err := ctx.HookStorage(); err == nil {
		*v.result = s.Tag()
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	return v, nil
}

// storageIdValue implements gnuflag.Value for use in storage commands.
type storageIdValue struct {
	result *names.StorageTag
	ctx    Context
}

// String returns the current value.
func (v *storageIdValue) String() string {
	if *v.result == (names.StorageTag{}) {
		return ""
	}
	return v.result.Id()
}

// Set interprets value as a storage id, if possible, and returns an error
// if it is not known to the system. The parsed storage id will be written
// to v.result.
func (v *storageIdValue) Set(value string) error {
	if !names.IsValidStorage(value) {
		return errors.Errorf("invalid storage ID %q", value)
	}
	tag := names.NewStorageTag(value)
	if _, err := v.ctx.Storage(tag); err != nil {
		return errors.Trace(err)
	}
	*v.result = tag
	return nil
}
