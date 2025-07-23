// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v10"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// RegisterImportCloudService registers the import operations with the given coordinator.
// It imports CloudServices for each application from model.
// Since a Cloud service is linked to an application and has ipaddresses to
// insert, we need to call it after both network and application import.
func RegisterImportCloudService(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importCloudServiceOperation{
		logger: logger,
	})
}

type importCloudServiceOperation struct {
	modelmigration.BaseOperation

	migrationService MigrationService
	logger           logger.Logger
}

// Name returns the name of this operation.
func (i *importCloudServiceOperation) Name() string {
	return "import cloud services"
}

// Setup implements Operation.
func (i *importCloudServiceOperation) Setup(scope modelmigration.Scope) error {
	st := state.NewState(scope.ModelDB(), i.logger)
	i.migrationService = service.NewMigrationService(st, i.logger)
	return nil
}

// Execute the import of the spaces, subnets and link layer devices
// contained in the model.
func (i *importCloudServiceOperation) Execute(ctx context.Context, model description.Model) error {
	if model.Type() != description.CAAS {
		return nil
	}

	cloudServices, err := i.collectServices(ctx, model)
	if err != nil {
		return errors.Errorf("collecting services and devices: %w", err)
	}

	if err := i.migrationService.ImportCloudServices(ctx, cloudServices); err != nil {
		return errors.Errorf("importing cloud services: %w", err)
	}

	return nil
}

func (i *importCloudServiceOperation) collectServices(
	ctx context.Context,
	model description.Model,
) ([]internal.ImportCloudService, error) {
	var cloudServices []internal.ImportCloudService
	for _, app := range model.Applications() {
		cloudService := app.CloudService()

		serviceUUID, err := uuid.NewUUID()
		if err != nil {
			return nil, errors.Errorf("creating service uuid: %w", err)
		}
		netNodeUUID, err := uuid.NewUUID()
		if err != nil {
			return nil, errors.Errorf("creating net node uuid: %w", err)
		}
		deviceUUID, err := uuid.NewUUID()
		if err != nil {
			return nil, errors.Errorf("creating device uuid: %w", err)
		}

		addresses, err := transform.SliceOrErr(cloudService.Addresses(),
			func(addr description.Address) (internal.ImportCloudServiceAddress, error) {
				addrUUID, err := uuid.NewUUID()
				if err != nil {
					return internal.ImportCloudServiceAddress{}, errors.Errorf("creating address uuid: %w", err)
				}

				return internal.ImportCloudServiceAddress{
					UUID:    addrUUID.String(),
					Value:   addr.Value(),
					Type:    addr.Type(),
					Scope:   addr.Scope(),
					Origin:  addr.Origin(),
					SpaceID: addr.SpaceID(),
				}, nil
			})
		if err != nil {
			return nil, errors.Errorf("transforming addresses: %w", err)
		}

		cloudServices = append(cloudServices, internal.ImportCloudService{
			UUID:            serviceUUID.String(),
			NetNodeUUID:     netNodeUUID.String(),
			DeviceUUID:      deviceUUID.String(),
			ApplicationName: app.Name(),
			ProviderID:      cloudService.ProviderId(),
			Addresses:       addresses,
		})
	}
	return cloudServices, nil
}
