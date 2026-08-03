// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

// ControllerModelInfoState describes the controller-database reads needed to
// export the controller-scoped portion of a model migration envelope.
type ControllerModelInfoState interface {
	// GetControllerModelInfo reads controller-database records scoped to the
	// given migrating model in target-portable semantic form.
	GetControllerModelInfo(
		ctx context.Context,
		modelUUID string,
		offerUUIDs []string,
		offererModels []coremodelmigration.OffererModel,
	) (coremodelmigration.ControllerModelInfo, error)
}

// ModelControllerInfoState describes model-database reads needed to select the
// controller-scoped records that travel with an exported model.
type ModelControllerInfoState interface {
	// GetOfferUUIDs returns UUIDs for offers hosted by this model.
	GetOfferUUIDs(ctx context.Context) ([]string, error)

	// GetThirdPartyOffererModels returns third-party (offerer controller,
	// offerer model) pairs referenced by this model.
	GetThirdPartyOffererModels(ctx context.Context) ([]coremodelmigration.OffererModel, error)
}

// ControllerInfoState wires the controller and model states used to export the
// controller-scoped model information.
type ControllerInfoState struct {
	Controller ControllerModelInfoState
	Model      ModelControllerInfoState
	ModelUUID  string
}

// GetControllerModelInfo reads the controller-database records scoped to this
// model and returns them in target-portable semantic form. It first reads the
// model's hosted offer UUIDs and third-party remote-offerer pairs from the
// model database, then reads the matching controller-database rows.
func (s *Service) GetControllerModelInfo(ctx context.Context) (coremodelmigration.ControllerModelInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if s.controllerInfo.Controller == nil || s.controllerInfo.Model == nil {
		return coremodelmigration.ControllerModelInfo{}, errors.Errorf("missing controller model info state")
	}

	offerUUIDs, err := s.controllerInfo.Model.GetOfferUUIDs(ctx)
	if err != nil {
		return coremodelmigration.ControllerModelInfo{}, errors.Errorf("reading model offer UUIDs: %w", err)
	}
	offererModels, err := s.controllerInfo.Model.GetThirdPartyOffererModels(ctx)
	if err != nil {
		return coremodelmigration.ControllerModelInfo{}, errors.Errorf("reading model offerer models: %w", err)
	}

	info, err := s.controllerInfo.Controller.GetControllerModelInfo(
		ctx, s.controllerInfo.ModelUUID, offerUUIDs, offererModels,
	)
	if err != nil {
		return coremodelmigration.ControllerModelInfo{}, errors.Errorf(
			"reading controller model info for %q: %w", s.controllerInfo.ModelUUID, err)
	}
	return info, nil
}
