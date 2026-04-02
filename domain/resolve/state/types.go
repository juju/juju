// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resolve"
	"github.com/juju/juju/internal/errors"
)

type unitUUID struct {
	UUID unit.UUID `db:"uuid"`
}

type unitName struct {
	Name unit.Name `db:"name"`
}

type unitResolveMode struct {
	Mode string `db:"mode"`
}

type statusID struct {
	StatusID int `db:"status_id"`
}

type unitResolve struct {
	UnitUUID unit.UUID `db:"unit_uuid"`
	ModeID   int       `db:"mode_id"`
}

func encodeResolveMode(mode resolve.ResolveMode) (int, error) {
	switch mode {
	case resolve.ResolveModeRetryHooks:
		return 0, nil
	case resolve.ResolveModeNoHooks:
		return 1, nil
	default:
		return -1, errors.Errorf("invalid resolve mode %q", mode)
	}
}
