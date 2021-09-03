// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import "github.com/juju/juju/state"

// SecretsWatcher instances watch for secret changes.
type SecretsWatcher interface {
	WatchSecretsRotationChanges(owner string) state.SecretsRotationWatcher
}
