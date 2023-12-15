// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import "github.com/juju/juju/core/lease"

const (
	MaxRetries         = maxRetries
	InitialRetryDelay  = initialRetryDelay
	RetryBackoffFactor = retryBackoffFactor
)

func ManagerStore(m *Manager) lease.Store {
	return m.config.Store
}
