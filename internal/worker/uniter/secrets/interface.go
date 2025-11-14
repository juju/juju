// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/juju/internal/worker/uniter/hook"
)

// SecretStateTracker provides access to the unit agent's
// state for secrets.
type SecretStateTracker interface {
	// PrepareHook returns an error if the hook is unknown
	// or invalid given current state.
	PrepareHook(hook.Info) error

	// CommitHook persists the state change encoded in the supplied secret
	// hook, or returns an error if the hook is unknown or invalid given
	// current secret state.
	CommitHook(info hook.Info) error

	// ConsumedSecretRevision returns the revision that
	// is currently tracked for the given secret.
	ConsumedSecretRevision(uri string) int

	// CollectRemovedSecretObsoleteRevisions takes the list of known obsolete
	// secrets and their revisions. It returns which secrets or revisions need
	// to be trimmed from the local secret state. Secrets where all the
	// revisions are to be trimmed have a length of 0. The returned value is
	// passed to SecretsRemoved.
	CollectRemovedSecretObsoleteRevisions(known map[string][]int) map[string][]int

	// SecretObsoleteRevisions returns the obsolete
	// revisions that have been reported already for
	// the given secret.
	SecretObsoleteRevisions(uri string) []int

	// SecretsRemoved updates the unit secrets state when secret revisions are
	// removed. DeletedRevisions will remove deleted secrets from both consumed
	// secrets state and obsolete secrets state. DeletedObsoleteRevisions will
	// remove deleted secrets from only the obsolete secret state. Both
	// arguments remove all revisions when the list of revisions has a length of
	// zero.
	SecretsRemoved(deletedRevisions, deletedObsoleteRevisions map[string][]int) error

	// Report provides information for the engine report.
	Report() map[string]interface{}
}

// Logger represents the logging methods used in this package.
type Logger interface {
	Warningf(string, ...interface{})
	Debugf(string, ...interface{})
}
