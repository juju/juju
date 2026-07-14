// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/export/types/latest"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	"github.com/juju/juju/internal/errors"
)

// reconcileSecretBackendUUIDs rewrites the model-DB payload's secret backend
// UUIDs from the source controller's to the target's, matched by backend name.
// It reads the target backends from the controller DB (read-only) and rewrites
// the in-memory payload in place; it runs after the controller data is imported
// and before the model-DB insert.
//
// It is the import-time counterpart to the activate-phase reconcile helpers:
// read controller-scoped data, reconcile the model's data. The reconciliation
// is a pre-insert payload rewrite (not a post-insert row update) because the
// target backends are already resolvable at import time — unlike source-
// controller CMR references, whose target values are only known at activation.
func reconcileSecretBackendUUIDs(
	ctx context.Context,
	deps Deps,
	info coremodelmigration.ControllerModelInfo,
	payload *latest.ModelExport,
) error {
	secretBackend := secretbackendservice.NewService(
		secretbackendstate.NewState(deps.ControllerDB, deps.Logger), deps.Logger,
	)
	revisionToTargetBackend, err := secretBackend.GetSecretBackendReferenceMapping(ctx, info.SecretBackendRefs)
	if err != nil {
		return errors.Errorf("resolving target secret backends: %w", err)
	}
	return rewriteSecretBackendUUIDs(payload, revisionToTargetBackend)
}

// rewriteSecretBackendUUIDs rewrites the transformed model-DB payload's
// secret_value_ref backend UUIDs from the source controller's backend UUIDs to
// the target's, keyed by secret revision UUID. revisionToTargetBackend maps
// each external-backed secret revision UUID to the target backend UUID resolved
// during the controller-data import.
//
// A value-ref revision with no mapping is a hard error (the model-DB insert has
// not run yet, so no rows leak). Deleted value refs are not rewritten here:
// source controllers remove secret_backend_reference rows when revisions move
// to secret_deleted_value_ref, so there is no revision-to-backend-name mapping
// available. The cleanup path only reads revision_id from those rows, so
// keeping them in the payload preserves the deferred external cleanup marker
// without inventing an impossible target backend mapping.
//
// The rewrite runs after the controller-data import and before any model-DB
// write. If it errors (missing mapping), the caller returns the error. The v8
// import returns a generic import failure, the source enters ABORT, and the
// abort path cleans the controller-DB data and the partial model. No model-DB
// rows were written, so the rewrite needs no compensation of its own.
func rewriteSecretBackendUUIDs(payload *latest.ModelExport, revisionToTargetBackend map[string]string) error {
	if payload == nil {
		return nil
	}

	if len(payload.SecretValueRef) == 0 {
		return nil
	}

	for i := range payload.SecretValueRef {
		rev := payload.SecretValueRef[i].RevisionUUID
		targetBackend, ok := revisionToTargetBackend[rev]
		if !ok {
			return errors.Errorf(
				"no target secret backend for secret revision %q", rev)
		}
		payload.SecretValueRef[i].BackendUUID = targetBackend
	}

	return nil
}
