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
	ModeID int `db:"mode_id"`
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

func decodeResolveMode(id int) (resolve.ResolveMode, error) {
	switch id {
	case 0:
		return resolve.ResolveModeRetryHooks, nil
	case 1:
		return resolve.ResolveModeNoHooks, nil
	default:
		return "", errors.Errorf("invalid resolve mode id %d", id)
	}
}
