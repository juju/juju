// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	uniterrors "github.com/juju/juju/domain/unit/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
)

// AtomicApplicationState describes retrieval and persistence methods for
// applications that require atomic transactions.
type AtomicApplicationState interface {
	domain.AtomicStateBase

	// GetApplicationID returns the ID for the named application, returning an
	// error satisfying [applicationerrors.ApplicationNotFound] if the
	// application is not found.
	GetApplicationID(ctx domain.AtomicContext, name string) (coreapplication.ID, error)

	// UpsertUnit creates or updates the specified application unit, returning
	// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application doesn't exist.
	UpsertUnit(domain.AtomicContext, coreapplication.ID, application.UpsertUnitArg) error

	// ApplicationScaleState looks up the scale state of the specified
	// application, returning an error satisfying
	// [applicationerrors.ApplicationNotFound] if the application is not found.
	ApplicationScaleState(domain.AtomicContext, coreapplication.ID) (application.ScaleState, error)

	// UnitLife looks up the life of the specified unit, returning an error
	// satisfying [uniterrors.NotFound] if the unit is not found.
	UnitLife(ctx domain.AtomicContext, unitName string) (life.Life, error)
}

// ApplicationState describes retrieval and persistence methods for
// applications.
type ApplicationState interface {
	AtomicApplicationState

	// StorageDefaults returns the default storage sources for a model.
	StorageDefaults(ctx context.Context) (domainstorage.StorageDefaults, error)

	// GetStoragePoolByName returns the storage pool with the specified name,
	// returning an error satisfying [storageerrors.PoolNotFoundError] if it
	// doesn't exist.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePoolDetails, error)

	// CreateApplication creates an application, whilst inserting a charm into
	// the database, returning an error satisfying
	// [applicationerrors.ApplicationAle\readyExists] if the application already
	// exists.
	CreateApplication(context.Context, string, application.AddApplicationArg, ...application.UpsertUnitArg) (coreapplication.ID, error)

	// DeleteApplication deletes the specified application, returning an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application doesn't exist. If the application still has units, as error
	// satisfying [applicationerrors.ApplicationHasUnits] is returned.
	DeleteApplication(context.Context, string) error

	// AddUnits adds the specified units to the application, returning an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application doesn't exist.
	AddUnits(ctx context.Context, applicationName string, args ...application.UpsertUnitArg) error
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
	// Some uses of application service don't need to supply a storage registry,
	// eg cleaner facade. In such cases it'd wasteful to create one as an
	// environ instance would be needed.
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

// CreateApplication creates the specified application and units if required,
// returning an error satisfying [applicationerrors.ApplicationAle\readyExists]
// if the application already exists.
func (s *ApplicationService) CreateApplication(
	ctx context.Context,
	name string,
	charm charm.Charm,
	origin corecharm.Origin,
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

	if err := origin.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", applicationerrors.CharmOriginNotValid, err)
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

	var channel *domaincharm.Channel
	if origin.Channel != nil {
		normalisedC := origin.Channel.Normalize()
		channel = &domaincharm.Channel{
			Track:  normalisedC.Track,
			Risk:   domaincharm.ChannelRisk(normalisedC.Risk),
			Branch: normalisedC.Branch,
		}
	}
	appArg := application.AddApplicationArg{
		Charm:   ch,
		Channel: channel,
		Platform: application.Platform{
			Channel:        origin.Platform.Channel,
			OSTypeID:       application.MarshallOSType(ostype.OSTypeForName(origin.Platform.OS)),
			ArchitectureID: application.MarshallArchitecture(origin.Platform.Architecture),
		},
	}

	unitArgs := make([]application.UpsertUnitArg, len(units))
	for i, u := range units {
		unitArgs[i] = makeUpsertUnitArgs(u)
	}

	id, err := s.st.CreateApplication(ctx, name, appArg, unitArgs...)
	if err != nil {
		return "", errors.Annotatef(err, "creating application %q", name)
	}
	return id, nil
}

func makeUpsertUnitArgs(in AddUnitArg) application.UpsertUnitArg {
	result := application.UpsertUnitArg{
		UnitName:     in.UnitName,
		PasswordHash: in.PasswordHash,
	}
	if in.CloudContainer != nil {
		result.CloudContainer = &application.CloudContainer{
			ProviderId: in.CloudContainer.ProviderId,
			Ports:      in.CloudContainer.Ports,
		}
		if in.CloudContainer.Address != nil {
			result.CloudContainer.Address = &application.Address{
				Value:       in.CloudContainer.Address.Value,
				AddressType: string(in.CloudContainer.Address.AddressType()),
				Scope:       string(in.CloudContainer.Address.Scope),
				SpaceID:     in.CloudContainer.Address.SpaceID,
				Origin:      string(network.OriginProvider),
			}
			if in.CloudContainer.AddressOrigin != nil {
				result.CloudContainer.Address.Origin = string(*in.CloudContainer.AddressOrigin)
			}
		}
	}
	return result
}

// AddUnits adds the specified units to the application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
func (s *ApplicationService) AddUnits(ctx context.Context, name string, units ...AddUnitArg) error {
	args := make([]application.UpsertUnitArg, len(units))
	for i, u := range units {
		args[i] = application.UpsertUnitArg{
			UnitName: u.UnitName,
		}
	}
	err := s.st.AddUnits(ctx, name, args...)
	return errors.Annotatef(err, "adding units to application %q", name)
}

// RegisterCAASUnit creates or updates the specified application unit in a caas model,
// returning an error satisfying [applicationerrors.ApplicationNotFoundError]
// if the application doesn't exist. If the application life is Dead, an error
// satisfying [applicationerrors.ApplicationIsDead] is returned.
func (s *ApplicationService) RegisterCAASUnit(ctx context.Context, appName string, args RegisterCAASUnitParams) error {
	if args.PasswordHash == nil {
		return errors.NotValidf("password hash")
	}
	if args.ProviderId == nil {
		return errors.NotValidf("provider id")
	}
	if !args.OrderedScale {
		return errors.NewNotImplemented(nil, "registering CAAS units not supported without ordered unit IDs")
	}
	if args.UnitName == "" {
		return errors.NotValidf("missing unit name")
	}

	p := AddUnitArg{
		UnitName:     &args.UnitName,
		PasswordHash: args.PasswordHash,
		CloudContainer: &CloudContainerParams{
			ProviderId: args.ProviderId,
			Ports:      args.Ports,
		},
	}
	if args.Address != nil {
		addr := network.NewSpaceAddress(*args.Address, network.WithScope(network.ScopeMachineLocal))
		// k8s doesn't support spaces yet.
		addr.SpaceID = network.AlphaSpaceId
		p.CloudContainer.Address = &addr
		origin := network.OriginProvider
		p.CloudContainer.AddressOrigin = &origin
	}
	// We need to do a bunch of business logic in the one transaction so pass in a closure that is
	// given the transaction to use.
	unitArg := makeUpsertUnitArgs(p)
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if err != nil {
			return errors.Trace(err)
		}
		unitLife, err := s.st.UnitLife(ctx, args.UnitName)
		if errors.Is(err, uniterrors.NotFound) {
			return s.insertCAASUnit(ctx, appID, args.OrderedId, unitArg)
		}
		if unitLife == life.Dead {
			return fmt.Errorf("dead unit %q already exists%w", args.UnitName, errors.Hide(applicationerrors.ApplicationIsDead))
		}
		return s.st.UpsertUnit(ctx, appID, unitArg)
	})
	return errors.Annotatef(err, "saving caas unit %q", args.UnitName)
}

func (s *ApplicationService) insertCAASUnit(
	ctx domain.AtomicContext, appID coreapplication.ID, orderedID int, args application.UpsertUnitArg,
) error {
	appScale, err := s.st.ApplicationScaleState(ctx, appID)
	// TODO(units) - need to wire up app scale updates
	appScale.Scale = 99999
	if err != nil {
		return errors.Annotatef(err, "getting application scale state for app %q", appID)
	}
	if orderedID >= appScale.Scale ||
		(appScale.Scaling && orderedID >= appScale.ScaleTarget) {
		return fmt.Errorf("unrequired unit %s is not assigned%w", *args.UnitName, errors.Hide(uniterrors.NotAssigned))
	}
	return s.st.UpsertUnit(ctx, appID, args)
}

// DeleteApplication deletes the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// If the application still has units, as error satisfying [applicationerrors.ApplicationHasUnits]
// is returned.
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
