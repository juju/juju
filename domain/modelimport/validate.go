// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/internal/errors"
)

// ValidatePayload validates target-version payload invariants that must hold
// before any target-side import writes proceed.
func ValidatePayload(payload latest.ModelExport) error {
	if len(payload.ModelAgent) != 1 {
		return errors.Errorf(
			"model export payload has %d model_agent rows, expected 1: %w",
			len(payload.ModelAgent), coreerrors.NotValid)
	}
	row := payload.ModelAgent[0]
	if row.PasswordHash == nil || *row.PasswordHash == "" {
		return errors.Errorf(
			"model export payload model_agent row has empty password_hash: %w",
			coreerrors.NotValid)
	}
	return nil
}
