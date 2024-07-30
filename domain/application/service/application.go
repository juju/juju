// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
)

// ApplicationState describes retrieval and persistence methods for
// applications.
type ApplicationState interface {
	// StorageDefaults returns the default storage sources for a model.
	StorageDefaults(ctx context.Context) (domainstorage.StorageDefaults, error)

	// GetStoragePoolByName returns the storage pool with the specified name, returning an error
	// satisfying [storageerrors.PoolNotFoundError] if it doesn't exist.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePoolDetails, error)

	// CreateApplication creates the specified application, with the charm, and
	// the units if required.
	CreateApplication(context.Context, string, domaincharm.Charm, ...application.AddUnitArg) (coreapplication.ID, error)

	// UpsertApplication persists the input Application entity.
	UpsertApplication(context.Context, string, ...application.AddUnitArg) error

	// DeleteApplication deletes the input Application entity.
	DeleteApplication(context.Context, string) error

	// AddUnits adds the specified units to the application.
	AddUnits(ctx context.Context, applicationName string, args ...application.AddUnitArg) error
}

// ApplicationService provides the API for working with applications.
type ApplicationService struct {
	st     ApplicationState
	logger logger.Logger

	registry  storage.ProviderRegistry
	modelType coremodel.ModelType
}

// NewApplicationService returns a new service reference wrapping the input state.
func NewApplicationService(st ApplicationState, registry storage.ProviderRegistry, logger logger.Logger) *ApplicationService {
	// Some uses of application service don't need to supply a storage registry, eg cleaner facade.
	// In such cases it'd wasteful to create one as an environ instance would be needed.
	if registry == nil {
		registry = storage.NotImplementedProviderRegistry{}
	}
	return &ApplicationService{
		st:       st,
		logger:   logger,
		registry: registry,
		// TODO(storage) - pass in model info getter
		modelType: coremodel.IAAS,
	}
}

// CreateApplication creates the specified application and units if required.
func (s *ApplicationService) CreateApplication(
	ctx context.Context,
	name string, charm charm.Charm,
	args AddApplicationArgs,
	units ...AddUnitArg,
) (coreapplication.ID, error) {
	// Validate that we have a valid charm and name.
	meta := charm.Meta()
	if meta == nil {
		return "", applicationerrors.CharmMetadataNotValid
	} else if name == "" && meta.Name == "" {
		return "", applicationerrors.ApplicationNameNotValid
	} else if meta.Name == "" {
		return "", applicationerrors.CharmNameNotValid
	}

	// We know that the charm name is valid, so we can use it as the application
	// name if that is not provided.
	if name == "" {
		name = meta.Name
	}

	// TODO (stickupkid): These should be done either in the application
	// state in one transaction, or be operating on the domain/charm types.
	//TODO(storage) - insert storage directive for app
	cons := make(map[string]storage.Directive)
	for n, sc := range args.Storage {
		cons[n] = sc
	}
	if err := s.addDefaultStorageDirectives(ctx, s.modelType, cons, meta); err != nil {
		return "", errors.Annotate(err, "adding default storage directives")
	}
	if err := s.validateStorageDirectives(ctx, s.modelType, cons, charm); err != nil {
		return "", errors.Annotate(err, "invalid storage directives")
	}

	// When encoding the charm, this will also validate the charm metadata,
	// when parsing it.
	ch, _, err := encodeCharm(charm)
	if err != nil {
		return "", fmt.Errorf("encode charm: %w", err)
	}

	unitArgs := make([]application.AddUnitArg, len(units))
	for i, u := range units {
		unitArgs[i] = application.AddUnitArg{
			UnitName: u.UnitName,
		}
	}

	id, err := s.st.CreateApplication(ctx, name, ch, unitArgs...)
	if err != nil {
		return "", errors.Annotatef(err, "creating application %q", name)
	}
	return id, nil
}

// AddUnits adds units to the application.
func (s *ApplicationService) AddUnits(ctx context.Context, name string, units ...AddUnitArg) error {
	args := make([]application.AddUnitArg, len(units))
	for i, u := range units {
		args[i] = application.AddUnitArg{
			UnitName: u.UnitName,
		}
	}
	err := s.st.AddUnits(ctx, name, args...)
	return errors.Annotatef(err, "adding units to application %q", name)
}

// UpsertCAASUnit records the existence of a unit in a caas model.
func (s *ApplicationService) UpsertCAASUnit(ctx context.Context, name string, unit UpsertCAASUnitParams) error {
	args := application.AddUnitArg{
		UnitName: unit.UnitName,
	}
	err := s.st.UpsertApplication(ctx, name, args)
	return errors.Annotatef(err, "saving application %q", name)
}

// DeleteApplication deletes the specified application.
func (s *ApplicationService) DeleteApplication(ctx context.Context, name string) error {
	err := s.st.DeleteApplication(ctx, name)
	return errors.Annotatef(err, "deleting application %q", name)
}

// UpdateApplicationCharm sets a new charm for the application, validating that aspects such
// as storage are still viable with the new charm.
func (s *ApplicationService) UpdateApplicationCharm(ctx context.Context, name string, params UpdateCharmParams) error {
	//TODO(storage) - update charm and storage directive for app
	return nil
}

// addDefaultStorageDirectives fills in default values, replacing any empty/missing values
// in the specified directives.
func (s *ApplicationService) addDefaultStorageDirectives(ctx context.Context, modelType coremodel.ModelType, allDirectives map[string]storage.Directive, charmMeta *charm.Meta) error {
	defaults, err := s.st.StorageDefaults(ctx)
	if err != nil {
		return errors.Annotate(err, "getting storage defaults")
	}
	return domainstorage.StorageDirectivesWithDefaults(charmMeta.Storage, modelType, defaults, allDirectives)
}

func (s *ApplicationService) validateStorageDirectives(ctx context.Context, modelType coremodel.ModelType, allDirectives map[string]storage.Directive, charm charm.Charm) error {
	validator, err := domainstorage.NewStorageDirectivesValidator(modelType, s.registry, s.st)
	if err != nil {
		return errors.Trace(err)
	}
	err = validator.ValidateStorageDirectivesAgainstCharm(ctx, allDirectives, charm)
	if err != nil {
		return errors.Trace(err)
	}
	// Ensure all stores have directives specified. Defaults should have
	// been set by this point, if the user didn't specify any.
	for name, charmStorage := range charm.Meta().Storage {
		if _, ok := allDirectives[name]; !ok && charmStorage.CountMin > 0 {
			return fmt.Errorf("%w for store %q", applicationerrors.MissingStorageDirective, name)
		}
	}
	return nil
}
