// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/charm/v13"
	"github.com/juju/errors"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/storage"
)

// Logger facilitates emitting log messages.
type Logger interface {
	Warningf(string, ...interface{})
}

// State describes retrieval and persistence methods for applications.
type State interface {
	// StorageDefaults returns the default storage sources for a model.
	StorageDefaults(ctx context.Context) (domainstorage.StorageDefaults, error)

	// GetStoragePoolByName returns the storage pool with the specified name, returning an error
	// satisfying [storageerrors.PoolNotFoundError] if it doesn't exist.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePoolDetails, error)

	// UpsertApplication persists the input Application entity.
	UpsertApplication(context.Context, string, ...application.AddUnitParams) error

	// DeleteApplication deletes the input Application entity.
	DeleteApplication(context.Context, string) error

	// AddUnits adds the specified units to the application.
	AddUnits(ctx context.Context, applicationName string, args ...application.AddUnitParams) error
}

// Service provides the API for working with applications.
type Service struct {
	st     State
	logger Logger

	registry  storage.ProviderRegistry
	modelType coremodel.ModelType
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, logger Logger, registry storage.ProviderRegistry) *Service {
	return &Service{
		st:       st,
		logger:   logger,
		registry: registry,
		// TODO(storage) - pass in model info getter
		modelType: coremodel.IAAS,
	}
}

// CreateApplication creates the specified application and units if required.
func (s *Service) CreateApplication(ctx context.Context, name string, params AddApplicationParams, units ...AddUnitParams) error {
	cons := make(map[string]storage.Constraints)
	for n, sc := range params.Storage {
		cons[n] = sc
	}
	if err := s.addDefaultStorageConstraints(ctx, s.modelType, cons, params.Charm.Meta()); err != nil {
		return errors.Annotate(err, "adding default storage constraints")
	}
	if err := s.validateStorageConstraints(ctx, s.modelType, cons, params.Charm); err != nil {
		return errors.Annotate(err, "invalid storage constraints")
	}
	args := make([]application.AddUnitParams, len(units))
	for i, u := range units {
		args[i] = application.AddUnitParams{
			UnitName: u.UnitName,
		}
	}
	//TODO(storage) - insert storage cons for app

	err := s.st.UpsertApplication(ctx, name, args...)
	return errors.Annotatef(err, "saving application %q", name)
}

// AddUnits adds units to the application.
func (s *Service) AddUnits(ctx context.Context, name string, units ...AddUnitParams) error {
	args := make([]application.AddUnitParams, len(units))
	for i, u := range units {
		args[i] = application.AddUnitParams{
			UnitName: u.UnitName,
		}
	}
	err := s.st.AddUnits(ctx, name, args...)
	return errors.Annotatef(err, "adding units to application %q", name)
}

// UpsertCAASUnit records the existence of a unit in a caas model.
func (s *Service) UpsertCAASUnit(ctx context.Context, name string, unit UpsertCAASUnitParams) error {
	args := application.AddUnitParams{
		UnitName: unit.UnitName,
	}
	err := s.st.UpsertApplication(ctx, name, args)
	return errors.Annotatef(err, "saving application %q", name)
}

// DeleteApplication deletes the specified application.
func (s *Service) DeleteApplication(ctx context.Context, name string) error {
	err := s.st.DeleteApplication(ctx, name)
	return errors.Annotatef(err, "deleting application %q", name)
}

// UpdateApplicationCharm sets a new charm for the application, validating that aspects such
// as storage are still viable with the new charm.
func (s *Service) UpdateApplicationCharm(ctx context.Context, name string, params UpdateCharmParams) error {
	//TODO(storage) - update charm and storage cons for app
	return nil
}

// addDefaultStorageConstraints fills in default constraint values, replacing any empty/missing values
// in the specified constraints.
func (s *Service) addDefaultStorageConstraints(ctx context.Context, modelType coremodel.ModelType, allCons map[string]storage.Constraints, charmMeta *charm.Meta) error {
	defaults, err := s.st.StorageDefaults(ctx)
	if err != nil {
		return errors.Annotate(err, "getting storage defaults")
	}
	return domainstorage.StorageConstraintsWithDefaults(charmMeta.Storage, modelType, defaults, allCons)
}

func (s *Service) validateStorageConstraints(ctx context.Context, modelType coremodel.ModelType, allCons map[string]storage.Constraints, charm Charm) error {
	validator := domainstorage.NewStorageConstraintsValidator(modelType, s.registry, s.st)
	err := validator.ValidateStorageConstraintsAgainstCharm(ctx, allCons, charm)
	if err != nil {
		return errors.Trace(err)
	}
	// Ensure all stores have constraints specified. Defaults should have
	// been set by this point, if the user didn't specify constraints.
	for name, charmStorage := range charm.Meta().Storage {
		if _, ok := allCons[name]; !ok && charmStorage.CountMin > 0 {
			return fmt.Errorf("%w for store %q", applicationerrors.MissingStorageConstraints, name)
		}
	}
	return nil
}
