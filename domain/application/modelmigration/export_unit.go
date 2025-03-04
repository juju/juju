// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/errors"
)

func (e *exportOperation) exportApplicationUnits(ctx context.Context, app description.Application) error {
	for _, unit := range app.Units() {
		unitName, err := coreunit.NewName(unit.Name())
		if err != nil {
			return errors.Errorf("parsing unit name %q: %v", unit.Name(), err)
		}

		workloadStatus, err := e.service.GetUnitWorkloadStatus(ctx, unitName)
		if err != nil {
			return errors.Errorf("getting unit workload status %q: %v", unitName, err)
		}
		unit.SetWorkloadStatus(e.exportStatus(workloadStatus))
	}

	return nil
}
