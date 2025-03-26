// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/version/v2"

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
		descriptionUnit := app.AddUnit(arg)

		// This is a hack to get the tools for the unit.
		descriptionUnit.SetTools(description.AgentToolsArgs{
			Version: version.Binary{
				Number: version.Number{
					Major: 4,
					Minor: 0,
					Tag:   "beta",
					Patch: 6,
					Build: 1,
				},
				Release: "ubuntu",
				Arch:    "amd64",
			},
			URL:    "tools/4.0-beta6.1-ubuntu-amd64-eb9e6949fd2fb1c4c6953619a4807ae1a3f54e355be1d37d14f5ba918fb07165",
			Size:   52806385,
			SHA256: "eb9e6949fd2fb1c4c6953619a4807ae1a3f54e355be1d37d14f5ba918fb07165",
		})
	}
	return nil
}

func (e *exportOperation) exportUnit(ctx context.Context, modelType string, unit application.ExportUnit) (description.UnitArgs, error) {
	args := description.UnitArgs{
		Name:         unit.Name.String(),
		Type:         modelType,
		PasswordHash: unit.PasswordHash,
	}

	return args, nil
}
