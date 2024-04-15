// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import (
	"time"

	"github.com/juju/juju/core/secrets"
)

// UpsertSecretParams are used to upsert a secret.
// Only non-nil values are used.
type UpsertSecretParams struct {
	RotatePolicy   *RotatePolicy
	ExpireTime     *time.Time
	NextRotateTime *time.Time
	Description    *string
	Label          *string
	AutoPrune      *bool

	Data     secrets.SecretData
	ValueRef *secrets.ValueRef
}

// HasUpdate returns true if at least one attribute to update is not nil.
func (u *UpsertSecretParams) HasUpdate() bool {
	return u.NextRotateTime != nil ||
		u.RotatePolicy != nil ||
		u.Description != nil ||
		u.Label != nil ||
		u.ExpireTime != nil ||
		len(u.Data) > 0 ||
		u.ValueRef != nil ||
		u.AutoPrune != nil
}
