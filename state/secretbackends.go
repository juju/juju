// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "time"

// CreateSecretBackendParams are used to create a secret backend.
type CreateSecretBackendParams struct {
	Name                string
	Backend             string
	TokenRotateInterval time.Duration
	Config              map[string]interface{}
}
