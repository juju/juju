// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"time"

	"github.com/juju/juju/v3/core/secrets"
	"github.com/juju/juju/v3/state"
)

// SecretsRotation instances provide secret rotation apis.
type SecretsRotation interface {
	WatchSecretsRotationChanges(owner string) state.SecretsRotationWatcher
	SecretRotated(url *secrets.URL, when time.Time) error
}
