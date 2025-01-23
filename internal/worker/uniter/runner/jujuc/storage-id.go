// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
)

// newStorageIdValue returns a gnuflag.Value for convenient parsing of storage
// ids in ctx.
func newStorageIdValue(ctx context.Context, cmdCtx Context, result *names.StorageTag) (*storageIdValue, error) {
	v := &storageIdValue{result: result, ctx: ctx, cmdCtx: cmdCtx}
	if s, err := cmdCtx.HookStorage(ctx); err == nil {
		*v.result = s.Tag()
	} else if !errors.Is(err, errors.NotFound) && !errors.Is(err, errors.NotProvisioned) {
		return nil, errors.Trace(err)
	}
	return v, nil
}

// storageIdValue implements gnuflag.Value for use in storage commands.
type storageIdValue struct {
	result *names.StorageTag
	ctx    context.Context
	cmdCtx Context
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
	if _, err := v.cmdCtx.Storage(v.ctx, tag); err != nil {
		return errors.Trace(err)
	}
	*v.result = tag
	return nil
}
