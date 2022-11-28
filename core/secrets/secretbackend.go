// Copyright 2022 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"time"
)

// SecretBackend defines a secrets backend.
type SecretBackend struct {
	Name                string
	Backend             string
	TokenRotateInterval time.Duration
	Config              map[string]interface{}
}
