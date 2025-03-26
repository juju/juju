// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/internal/errors"
)

func (e *exportOperation) exportApplicationUnits(ctx context.Context, app description.Application) error {
	units, err := e.service.GetApplicationUnits(ctx, app.Name())
	if err != nil {
		return errors.Capture(err)
	}

	modelType := app.Type()

	for _, unit := range units {
		arg, err := e.exportUnit(ctx, modelType, unit)
		if err != nil {
			return errors.Capture(err)
		}
		app.AddUnit(arg)
	}
	return nil
}

func (e *exportOperation) exportUnit(ctx context.Context, modelType string, unit application.ExportUnit) (description.UnitArgs, error) {
	return description.UnitArgs{
		Name:         unit.Name.String(),
		Type:         modelType,
		PasswordHash: unit.PasswordHash,
	}, nil
}
