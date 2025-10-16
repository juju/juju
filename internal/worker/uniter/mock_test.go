// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/rpc/params"
)

type dummyStorageAccessor struct {
	api.StorageAccessor
}

func (*dummyStorageAccessor) UnitStorageAttachments(ctx context.Context, _ names.UnitTag) ([]params.StorageAttachmentId, error) {
	return nil, nil
}

type dummySecretsAccessor struct {
	api.SecretsClient
}

func (a *dummySecretsAccessor) SecretMetadata(context.Context) ([]secrets.SecretMetadata, error) {
	return nil, nil
}

func (*dummySecretsAccessor) GetConsumerSecretsRevisionInfo(context.Context, string, []string) (map[string]secrets.SecretRevisionInfo, error) {
	return nil, nil
}

type nopResolver struct{}

func (nopResolver) NextOp(context.Context, resolver.LocalState, remotestate.Snapshot, operation.Factory) (operation.Operation, error) {
	return nil, resolver.ErrNoOperation
}
