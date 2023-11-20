// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/names/v4"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/uniter/api"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

type dummyStorageAccessor struct {
	api.StorageAccessor
}

func (*dummyStorageAccessor) UnitStorageAttachments(_ names.UnitTag) ([]params.StorageAttachmentId, error) {
	return nil, nil
}

type dummySecretsAccessor struct {
	api.SecretsClient
}

func (a *dummySecretsAccessor) SecretMetadata() ([]secrets.SecretOwnerMetadata, error) {
	return nil, nil
}

func (*dummySecretsAccessor) GetConsumerSecretsRevisionInfo(string, []string) (map[string]secrets.SecretRevisionInfo, error) {
	return nil, nil
}

type nopResolver struct{}

func (nopResolver) NextOp(context.Context, resolver.LocalState, remotestate.Snapshot, operation.Factory) (operation.Operation, error) {
	return nil, resolver.ErrNoOperation
}
