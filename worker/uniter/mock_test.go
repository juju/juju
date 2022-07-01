// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/juju/v3/rpc/params"
	"github.com/juju/juju/v3/worker/uniter/operation"
	"github.com/juju/juju/v3/worker/uniter/remotestate"
	"github.com/juju/juju/v3/worker/uniter/resolver"
	"github.com/juju/juju/v3/worker/uniter/storage"
	"github.com/juju/names/v4"
)

type dummyStorageAccessor struct {
	storage.StorageAccessor
}

func (*dummyStorageAccessor) UnitStorageAttachments(_ names.UnitTag) ([]params.StorageAttachmentId, error) {
	return nil, nil
}

type nopResolver struct{}

func (nopResolver) NextOp(resolver.LocalState, remotestate.Snapshot, operation.Factory) (operation.Operation, error) {
	return nil, resolver.ErrNoOperation
}
