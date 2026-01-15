// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker

import (
	"time"

	"github.com/juju/juju/state"
)

type SecretsState interface {
	WatchSecretBackendIssuedTokenExpiry() state.StringsWatcher
	ListSecretBackendIssuedTokenUntil(
		until time.Time,
	) ([]state.SecretBackendIssuedToken, error)
	RemoveSecretBackendIssuedTokens(uuids []string) error
}
