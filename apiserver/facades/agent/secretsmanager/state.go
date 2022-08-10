// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"time"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/state"
)

// SecretsRotation instances provide secret rotation apis.
type SecretsRotation interface {
	WatchSecretsRotationChanges(owner string) state.SecretsRotationWatcher
	SecretRotated(uri *secrets.URI, when time.Time) error
}

// SecretsConsumer instances provide secret consumer apis.
type SecretsConsumer interface {
	GetSecretConsumer(*secrets.URI, string) (*secrets.SecretConsumerMetadata, error)
	SaveSecretConsumer(*secrets.URI, string, *secrets.SecretConsumerMetadata) error
	WatchConsumedSecretsChanges(string) state.StringsWatcher
}
