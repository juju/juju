// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v12"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// RegisterImportK8sService registers the import operations with the given coordinator.
// It imports K8sServices for each application from model.
// Since a K8s service is linked to an application and has ipaddresses to
// insert, we need to call it after both network and application import.
func RegisterImportK8sService(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importK8sServiceOperation{
		logger: logger,
	})
}

// K8sServiceMigrationService defines methods needed to import
// cloud services as part of model migration.
type K8sServiceMigrationService interface {
	// ImportK8sServices imports cloud service metadata into the model using the provided context and service data.
	ImportK8sServices(ctx context.Context, services []internal.ImportK8sService) error
}

type importK8sServiceOperation struct {
	modelmigration.BaseOperation[description.Model]

	migrationService K8sServiceMigrationService
	logger           logger.Logger
}

// Name returns the name of this operation.
func (i *importK8sServiceOperation) Name() string {
	return "import cloud services"
}

// Setup implements Operation.
func (i *importK8sServiceOperation) Setup(scope modelmigration.Scope) error {
	st := state.NewState(scope.ModelDB(), i.logger)
	i.migrationService = service.NewMigrationService(st, i.logger)
	return nil
}

// Execute runs the operation for importing cloud services into the model,
// ensuring the model type is CAAS. If the model type is not CAAS,
// the method exits without action.
// Collects cloud services from the provided model and
// imports them using MigrationService.
// Returns an error if the collection or import process fails.
func (i *importK8sServiceOperation) Execute(ctx context.Context, model description.Model) error {
	if model.Type() != description.CAAS {
		return nil
	}

	k8sServices, err := i.encodeK8sServices(model)
	if err != nil {
		return errors.Errorf("collecting services and devices: %w", err)
	}

	if err := i.migrationService.ImportK8sServices(ctx, k8sServices); err != nil {
		return errors.Errorf("importing cloud services: %w", err)
	}

	return nil
}

func (i *importK8sServiceOperation) encodeK8sServices(
	model description.Model,
) ([]internal.ImportK8sService, error) {
	var k8sServices []internal.ImportK8sService
	for _, app := range model.Applications() {
		k8sService := app.CloudService()
		if k8sService == nil {
			continue
		}

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

		addresses, err := transform.SliceOrErr(k8sService.Addresses(),
			func(addr description.Address) (internal.ImportK8sServiceAddress, error) {
				addrUUID, err := uuid.NewUUID()
				if err != nil {
					return internal.ImportK8sServiceAddress{}, errors.Errorf("creating address uuid: %w", err)
				}

				return internal.ImportK8sServiceAddress{
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

		k8sServices = append(k8sServices, internal.ImportK8sService{
			UUID:            serviceUUID.String(),
			NetNodeUUID:     netNodeUUID.String(),
			DeviceUUID:      deviceUUID.String(),
			ApplicationName: app.Name(),
			ProviderID:      k8sService.ProviderId(),
			Addresses:       addresses,
		})
	}
	return k8sServices, nil
}
