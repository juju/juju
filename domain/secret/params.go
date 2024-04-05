// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import (
	"time"

	"github.com/juju/juju/core/secrets"
)

// UpsertSecretParams are used to upsert a secret.
type UpsertSecretParams struct {
	RotatePolicy secrets.RotatePolicy
	ExpireTime   time.Time
	Description  string
	Label        string
	Data         secrets.SecretData
	ValueRef     *secrets.ValueRef
	AutoPrune    bool
}
