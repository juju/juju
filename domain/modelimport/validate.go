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
	appNames := make(map[string]string, len(payload.Application))
	for _, app := range payload.Application {
		appNames[app.UUID] = app.Name
	}
	aliveUnits := make(map[string]int)
	for _, unit := range payload.Unit {
		if unit.LifeID == 0 {
			aliveUnits[unit.ApplicationUUID]++
		}
	}
	for _, scale := range payload.ApplicationScale {
		name, ok := appNames[scale.ApplicationUUID]
		if !ok {
			return errors.Errorf("imported scale has no application %q: %w", scale.ApplicationUUID, coreerrors.NotValid)
		}
		if scale.Scale != nil && *scale.Scale != int64(aliveUnits[scale.ApplicationUUID]) {
			return errors.Errorf("imported application %q has scale %d but %d Alive units: scale request is not materialised: %w",
				name, *scale.Scale, aliveUnits[scale.ApplicationUUID], coreerrors.NotValid)
		}
	}
	return nil
}
