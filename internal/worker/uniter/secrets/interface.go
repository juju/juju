// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/juju/internal/worker/uniter/hook"
)

// SecretStateTracker provides access to the unit agent's
// state for secrets.
type SecretStateTracker interface {
	// PrepareHook returns an error if the hook is unknown
	// or invalid given current state.
	PrepareHook(context.Context, hook.Info) error

	// CommitHook persists the state change encoded in the supplied secret
	// hook, or returns an error if the hook is unknown or invalid given
	// current secret state.
	CommitHook(_ context.Context, info hook.Info) error

	// ConsumedSecretRevision returns the revision that
	// is currently tracked for the given secret.
	ConsumedSecretRevision(uri string) int

	// SecretObsoleteRevisions returns the obsolete
	// revisions that have been reported already for
	// the given secret.
	SecretObsoleteRevisions(uri string) []int

	// SecretsRemoved updates the unit secrets state
	// when secrets are removed.
	SecretsRemoved(uris []string) error

	// Report provides information for the engine report.
	Report() map[string]interface{}
}
