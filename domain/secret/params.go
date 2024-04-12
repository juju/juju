// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import (
	"time"

	"github.com/juju/juju/core/secrets"
)

// UpsertSecretParams are used to upsert a secret.
type UpsertSecretParams struct {
	RotatePolicy   *RotatePolicy
	ExpireTime     *time.Time
	NextRotateTime *time.Time
	// TODO(secrets) - these should be pointers
	Description string
	Label       string
	AutoPrune   bool

	Data     secrets.SecretData
	ValueRef *secrets.ValueRef
}
