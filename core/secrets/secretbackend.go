// Copyright 2022 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"time"
)

// SecretBackend defines a secrets backend.
type SecretBackend struct {
	Name                string
	BackendType         string
	TokenRotateInterval *time.Duration
	Config              map[string]interface{}
}
