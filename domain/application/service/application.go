// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/leadership"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	secretservice "github.com/juju/juju/domain/secret/service"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs"
	internalcharm "github.com/juju/juju/internal/charm"
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

	// GetApplicationLife looks up the life of the specified application, returning an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the application is not found.
	GetApplicationLife(ctx domain.AtomicContext, appName string) (coreapplication.ID, life.Life, error)

	// SetApplicationLife sets the life of the specified application.
	SetApplicationLife(ctx domain.AtomicContext, appID coreapplication.ID, l life.Life) error

	// GetApplicationScaleState looks up the scale state of the specified
	// application, returning an error satisfying
	// [applicationerrors.ApplicationNotFound] if the application is not found.
	GetApplicationScaleState(domain.AtomicContext, coreapplication.ID) (application.ScaleState, error)

	// SetApplicationScalingState sets the scaling details for the given caas application
	// Scale is optional and is only set if not nil.
	SetApplicationScalingState(ctx domain.AtomicContext, appID coreapplication.ID, scale *int, targetScale int, scaling bool) error

	// SetDesiredApplicationScale updates the desired scale of the specified application.
	SetDesiredApplicationScale(ctx domain.AtomicContext, appID coreapplication.ID, scale int) error

	// GetUnitLife looks up the life of the specified unit, returning an error
	// satisfying [applicationerrors.UnitNotFound] if the unit is not found.
	GetUnitLife(ctx domain.AtomicContext, unitName string) (life.Life, error)

	// SetUnitLife sets the life of the specified unit.
	SetUnitLife(ctx domain.AtomicContext, unitName string, life life.Life) error

	// InitialWatchStatementUnitLife returns the initial namespace query for the application unit life watcher.
	InitialWatchStatementUnitLife(appName string) (string, eventsource.NamespaceQuery)

	// DeleteApplication deletes the specified application, returning an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application doesn't exist. If the application still has units, as error
	// satisfying [applicationerrors.ApplicationHasUnits] is returned.
	DeleteApplication(domain.AtomicContext, string) error

	// DeleteUnit deletes the specified unit.
	// If the unit's application is Dying and no
	// other references to it exist, true is returned to
	// indicate the application could be safely deleted.
	// It will fail if the unit is not Dead.
	DeleteUnit(ctx domain.AtomicContext, unitName string) (bool, error)
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
	// [applicationerrors.ApplicationAlreadyExists] if the application already
	// exists.
	CreateApplication(context.Context, string, application.AddApplicationArg, ...application.UpsertUnitArg) (coreapplication.ID, error)

	// AddUnits adds the specified units to the application, returning an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application doesn't exist.
	AddUnits(ctx context.Context, applicationName string, args ...application.UpsertUnitArg) error

	// UpsertCloudService updates the cloud service for the specified
	// application, returning an error satisfying
	// [applicationerrors.ApplicationNotFoundError] if the application doesn't
	// exist.
	UpsertCloudService(ctx context.Context, appName, providerID string, sAddrs network.SpaceAddresses) error

	// GetApplicationUnitLife returns the life values for the specified units of
	// the given application. The supplied ids may belong to a different
	// application; the application name is used to filter.
	GetApplicationUnitLife(ctx context.Context, appName string, unitIDs ...string) (map[string]life.Life, error)

	// GetCharmByApplicationName returns the charm and the charm origin for the
	// specified application name.
	// If the application does not exist, an error satisfying
	// [applicationerrors.ApplicationNotFoundError] is returned.
	// If the charm for the application does not exist, an error satisfying
	// [applicationerrors.CharmNotFoundError] is returned.
	GetCharmByApplicationName(context.Context, string) (domaincharm.Charm, domaincharm.CharmOrigin, application.Platform, error)
}

const (
	// applicationSnippet is a non-compiled regexp that can be composed with
	// other snippets to form a valid application regexp.
	applicationSnippet = "(?:[a-z][a-z0-9]*(?:-[a-z0-9]*[a-z][a-z0-9]*)*)"
)

var (
	validApplication = regexp.MustCompile("^" + applicationSnippet + "$")
)

// SecretService defines methods on a secret service
// used by an application service.
type SecretService interface {
	InternalDeleteSecret(ctx domain.AtomicContext, uri *coresecrets.URI, params secretservice.DeleteSecretParams) (func(context.Context), error)
	GetSecretsForOwners(ctx domain.AtomicContext, owners ...secretservice.CharmSecretOwner) ([]*coresecrets.URI, error)
}

// NotImplementedSecretService defines a secret service which does nothing.
type NotImplementedSecretService struct{}

func (s NotImplementedSecretService) GetSecretsForOwners(ctx domain.AtomicContext, owners ...secretservice.CharmSecretOwner) ([]*coresecrets.URI, error) {
	return nil, nil
}

func (NotImplementedSecretService) InternalDeleteSecret(domain.AtomicContext, *coresecrets.URI, secretservice.DeleteSecretParams) (func(context.Context), error) {
	return func(context.Context) {}, nil
}

// ApplicationService provides the API for working with applications.
type ApplicationService struct {
	st     ApplicationState
	logger logger.Logger

	modelType coremodel.ModelType

	registry      storage.ProviderRegistry
	secretService SecretService
}

// NewApplicationService returns a new service reference wrapping the input state.
func NewApplicationService(st ApplicationState, params ApplicationServiceParams, logger logger.Logger) *ApplicationService {
	// Some uses of application service don't need to supply a storage registry,
	// eg cleaner facade. In such cases it'd wasteful to create one as an
	// environ instance would be needed.
	if params.StorageRegistry == nil {
		params.StorageRegistry = storage.NotImplementedProviderRegistry{}
	}
	if params.Secrets == nil {
		params.Secrets = NotImplementedSecretService{}
	}
	return &ApplicationService{
		st:            st,
		logger:        logger,
		registry:      params.StorageRegistry,
		secretService: params.Secrets,
		// TODO(storage) - pass in model info getter
		modelType: coremodel.IAAS,
	}
}

// CreateApplication creates the specified application and units if required,
// returning an error satisfying [applicationerrors.ApplicationAlreadyExists]
// if the application already exists.
func (s *ApplicationService) CreateApplication(
	ctx context.Context,
	name string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
	units ...AddUnitArg,
) (coreapplication.ID, error) {
	if !isValidApplicationName(name) {
		return "", applicationerrors.ApplicationNameNotValid
	}

	// Validate that we have a valid charm and name.
	meta := charm.Meta()
	if meta == nil {
		return "", applicationerrors.CharmMetadataNotValid
	}

	if !isValidCharmName(meta.Name) {
		return "", applicationerrors.CharmNameNotValid
	} else if !isValidReferenceName(args.ReferenceName) {
		return "", fmt.Errorf("reference name: %w", applicationerrors.CharmNameNotValid)
	}

	// Validate the origin of the charm.
	if err := origin.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", applicationerrors.CharmOriginNotValid, err)
	}

	// We know that the charm name is valid, so we can use it as the application
	// name if that is not provided.
	if name == "" {
		// Annoyingly this should be the reference name, but that's not
		// true in the previous code. To keep compatibility, we'll use the
		// charm name.
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

	originArg, channelArg, platformArg, err := encodeCharmOrigin(origin, args.ReferenceName)
	if err != nil {
		return "", fmt.Errorf("encode charm origin: %w", err)
	}

	appArg := application.AddApplicationArg{
		Charm:    ch,
		Platform: platformArg,
		Origin:   originArg,
		Channel:  channelArg,
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

// GetUnitLife looks up the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFoundError] if the unit is not found.
func (s *ApplicationService) GetUnitLife(ctx context.Context, unitName string) (corelife.Value, error) {
	var result corelife.Value
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		unitLife, err := s.st.GetUnitLife(ctx, unitName)
		result = unitLife.Value()
		return errors.Annotatef(err, "getting life for %q", unitName)
	})
	return result, errors.Trace(err)
}

// DeleteUnit deletes the specified unit.
// TODO(units) - rework when dual write is refactored
// This method is called (mostly during cleanup) after a unit
// has been removed from mongo. The mongo calls are
// DestroyMaybeRemove, DestroyWithForce, RemoveWithForce.
func (s *ApplicationService) DeleteUnit(ctx context.Context, unitName string) error {
	var cleanups []func(context.Context)
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		cleanups, err = s.deleteUnit(ctx, unitName)
		return errors.Trace(err)
	})
	if err != nil {
		return errors.Annotatef(err, "deleting unit %q", unitName)
	}
	for _, cleanup := range cleanups {
		cleanup(ctx)
	}
	return nil
}

func (s *ApplicationService) deleteUnit(ctx domain.AtomicContext, unitName string) ([]func(context.Context), error) {
	// Get unit owned secrets.
	uris, err := s.secretService.GetSecretsForOwners(ctx, secretservice.CharmSecretOwner{
		Kind: secretservice.UnitOwner,
		ID:   unitName,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "getting unit owned secrets for %q", unitName)
	}
	// Delete unit owned secrets.
	var cleanups []func(context.Context)
	for _, uri := range uris {
		s.logger.Debugf("deleting unit %q secret: %s", unitName, uri.ID)
		cleanup, err := s.secretService.InternalDeleteSecret(ctx, uri, secretservice.DeleteSecretParams{
			Accessor: secretservice.SecretAccessor{
				Kind: secretservice.UnitAccessor,
				ID:   unitName,
			},
		})
		if err != nil {
			return nil, errors.Annotatef(err, "deleting secret %q", uri)
		}
		cleanups = append(cleanups, cleanup)
	}

	err = s.ensureUnitDead(ctx, unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	isLast, err := s.st.DeleteUnit(ctx, unitName)
	if err != nil {
		return nil, errors.Annotatef(err, "deleting unit %q", unitName)
	}
	if isLast {
		// TODO(units): schedule application cleanup
		_ = isLast
	}
	return cleanups, nil
}

// DestroyUnit prepares a unit for removal from the model
// returning an error  satisfying [applicationerrors.UnitNotFoundError]
// if the unit doesn't exist.
func (s *ApplicationService) DestroyUnit(ctx context.Context, unitName string) error {
	// For now, all we do is advance the unit's life to Dying.
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return s.st.SetUnitLife(ctx, unitName, life.Dying)
	})
	return errors.Annotatef(err, "destroying unit %q", unitName)
}

// EnsureUnitDead is called by the unit agent just before it terminates.
// TODO(units): revisit his existing logic ported from mongo
// Note: the agent only calls this method once it gets notification
// that the unit has become dead, so there's strictly no need to call
// this method as the unit is already dead.
// This method is also called during cleanup from various cleanup jobs.
// If the unit is not found, an error satisfying [applicationerrors.UnitNotFound]
// is returned.
func (s *ApplicationService) EnsureUnitDead(ctx context.Context, unitName string, leadershipRevoker leadership.Revoker) error {
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return s.ensureUnitDead(ctx, unitName)
	})
	if err == nil {
		appName, _ := names.UnitApplication(unitName)
		if err := leadershipRevoker.RevokeLeadership(appName, unitName); err != nil && !errors.Is(err, leadership.ErrClaimNotHeld) {
			s.logger.Warningf("cannot revoke lease for dead unit %q", unitName)
		}
	}
	return errors.Annotatef(err, "ensuring unit %q is dead", unitName)
}

func (s *ApplicationService) ensureUnitDead(ctx domain.AtomicContext, unitName string) (err error) {
	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return errors.Trace(err)
	}
	if unitLife == life.Dead {
		return nil
	}
	// TODO(units) - check for subordinates and storage attachments
	// For IAAS units, we need to do additional checks - these are still done in mongo.
	// If a unit still has subordinates, return applicationerrors.UnitHasSubordinates.
	// If a unit still has storage attachments, return applicationerrors.UnitHasStorageAttachments.
	err = s.st.SetUnitLife(ctx, unitName, life.Dead)
	return errors.Annotatef(err, "ensuring unit %q is dead", unitName)
}

// RemoveUnit is called by the deployer worker and caas application provisioner worker to
// remove from the model units which have transitioned to dead.
// TODO(units): revisit his existing logic ported from mongo
// Note: the callers of this method only do so after the unit has become dead, so
// there's strictly no need to call ensureUnitDead before removing.
// If the unit is still alive, an error satisfying [applicationerrors.UnitIsAlive]
// is returned. If the unit is not found, an error satisfying
// [applicationerrors.UnitNotFound] is returned.
func (s *ApplicationService) RemoveUnit(ctx context.Context, unitName string, leadershipRevoker leadership.Revoker) error {
	var cleanups []func(context.Context)
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		unitLife, err := s.st.GetUnitLife(ctx, unitName)
		if err != nil {
			return errors.Trace(err)
		}
		if unitLife == life.Alive {
			return fmt.Errorf("cannot remove unit %q: %w", unitName, applicationerrors.UnitIsAlive)
		}
		cleanups, err = s.deleteUnit(ctx, unitName)
		return errors.Annotatef(err, "deleting unit %q", unitName)
	})
	if err != nil {
		return errors.Annotatef(err, "removing unit %q", unitName)
	}
	appName, _ := names.UnitApplication(unitName)
	if err := leadershipRevoker.RevokeLeadership(appName, unitName); err != nil && !errors.Is(err, leadership.ErrClaimNotHeld) {
		s.logger.Warningf("cannot revoke lease for dead unit %q", unitName)
	}
	for _, cleanup := range cleanups {
		cleanup(ctx)
	}
	return nil
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
		unitLife, err := s.st.GetUnitLife(ctx, args.UnitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			return s.insertCAASUnit(ctx, appID, args.OrderedId, unitArg)
		}
		if unitLife == life.Dead {
			return fmt.Errorf("dead unit %q already exists%w", args.UnitName, errors.Hide(applicationerrors.UnitAlreadyExists))
		}
		return s.st.UpsertUnit(ctx, appID, unitArg)
	})
	return errors.Annotatef(err, "saving caas unit %q", args.UnitName)
}

func (s *ApplicationService) insertCAASUnit(
	ctx domain.AtomicContext, appID coreapplication.ID, orderedID int, args application.UpsertUnitArg,
) error {
	appScale, err := s.st.GetApplicationScaleState(ctx, appID)
	if err != nil {
		return errors.Annotatef(err, "getting application scale state for app %q", appID)
	}
	if orderedID >= appScale.Scale ||
		(appScale.Scaling && orderedID >= appScale.ScaleTarget) {
		return fmt.Errorf("unrequired unit %s is not assigned%w", *args.UnitName, errors.Hide(applicationerrors.UnitNotAssigned))
	}
	return s.st.UpsertUnit(ctx, appID, args)
}

// DeleteApplication deletes the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// If the application still has units, as error satisfying [applicationerrors.ApplicationHasUnits]
// is returned.
func (s *ApplicationService) DeleteApplication(ctx context.Context, name string) error {
	var cleanups []func(context.Context)
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		cleanups, err = s.deleteApplication(ctx, name)
		return errors.Trace(err)
	})
	if err != nil {
		return errors.Annotatef(err, "deleting application %q", name)
	}
	for _, cleanup := range cleanups {
		cleanup(ctx)
	}
	return nil
}

func (s *ApplicationService) deleteApplication(ctx domain.AtomicContext, name string) ([]func(context.Context), error) {
	// Get app owned secrets.
	uris, err := s.secretService.GetSecretsForOwners(ctx, secretservice.CharmSecretOwner{
		Kind: secretservice.ApplicationOwner,
		ID:   name,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "getting application owned secrets for %q", name)
	}
	// Delete app owned secrets.
	var cleanups []func(context.Context)
	for _, uri := range uris {
		s.logger.Debugf("deleting application %q secret: %s", name, uri.ID)
		cleanup, err := s.secretService.InternalDeleteSecret(ctx, uri, secretservice.DeleteSecretParams{
			// TODO(units) - access is expected to be a unit
			// It is passed to CleanupSecrets() on k8s and is used
			// to name the service account..
			Accessor: secretservice.SecretAccessor{
				Kind: secretservice.UnitAccessor,
				ID:   name + "/0",
			},
		})
		if err != nil {
			return nil, errors.Annotatef(err, "deleting secret %q", uri)
		}
		cleanups = append(cleanups, cleanup)
	}

	err = s.st.DeleteApplication(ctx, name)
	if err != nil {
		return nil, errors.Annotatef(err, "deleting application %q", name)
	}
	return cleanups, nil
}

// DestroyApplication prepares an application for removal from the model
// returning an error  satisfying [applicationerrors.ApplicationNotFoundError]
// if the application doesn't exist.
func (s *ApplicationService) DestroyApplication(ctx context.Context, appName string) error {
	// For now, all we do is advance the application's life to Dying.
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if errors.Is(err, applicationerrors.ApplicationIsDead) {
			return nil
		}
		if err != nil {
			return errors.Trace(err)
		}
		return s.st.SetApplicationLife(ctx, appID, life.Dying)
	})
	return errors.Annotatef(err, "destroying application %q", appName)
}

// EnsureApplicationDead is called by the cleanup worker if a mongo
// destroy operation sets the application to dead.
// TODO(units): remove when everything is in dqlite.
func (s *ApplicationService) EnsureApplicationDead(ctx context.Context, appName string) error {
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if errors.Is(err, applicationerrors.ApplicationIsDead) {
			return nil
		}
		if err != nil {
			return errors.Trace(err)
		}
		return s.st.SetApplicationLife(ctx, appID, life.Dead)
	})
	return errors.Annotatef(err, "setting application %q life to Dead", appName)
}

// UpdateApplicationCharm sets a new charm for the application, validating that aspects such
// as storage are still viable with the new charm.
func (s *ApplicationService) UpdateApplicationCharm(ctx context.Context, name string, params UpdateCharmParams) error {
	//TODO(storage) - update charm and storage directive for app
	return nil
}

// GetCharmByApplicationName returns the charm for the specified application
// name.
// If the application does not exist, an error satisfying
// [applicationerrors.ApplicationNotFoundError] is returned.
// If the charm for the application does not exist, an error satisfying
// [applicationerrors.CharmNotFoundError] is returned.
// If the application name is not valid, an error satisfying
// [applicationerrors.ApplicationNameNotValid] is returned.
func (s *ApplicationService) GetCharmByApplicationName(ctx context.Context, name string) (
	internalcharm.Charm,
	domaincharm.CharmOrigin,
	application.Platform,
	error,
) {
	if !isValidApplicationName(name) {
		return nil, domaincharm.CharmOrigin{}, application.Platform{}, applicationerrors.ApplicationNameNotValid
	}

	charm, origin, platform, err := s.st.GetCharmByApplicationName(ctx, name)
	if err != nil {
		return nil, origin, platform, errors.Trace(err)
	}

	// The charm needs to be decoded into the internalcharm.Charm type.

	metadata, err := decodeMetadata(charm.Metadata)
	if err != nil {
		return nil, origin, platform, errors.Trace(err)
	}

	manifest, err := decodeManifest(charm.Manifest)
	if err != nil {
		return nil, origin, platform, errors.Trace(err)
	}

	actions, err := decodeActions(charm.Actions)
	if err != nil {
		return nil, origin, platform, errors.Trace(err)
	}

	config, err := decodeConfig(charm.Config)
	if err != nil {
		return nil, origin, platform, errors.Trace(err)
	}

	lxdProfile, err := decodeLXDProfile(charm.LXDProfile)
	if err != nil {
		return nil, origin, platform, errors.Trace(err)
	}

	return internalcharm.NewCharmBase(
		&metadata,
		&manifest,
		&config,
		&actions,
		&lxdProfile,
	), origin, platform, nil
}

// addDefaultStorageDirectives fills in default values, replacing any empty/missing values
// in the specified directives.
func (s *ApplicationService) addDefaultStorageDirectives(ctx context.Context, modelType coremodel.ModelType, allDirectives map[string]storage.Directive, charmMeta *internalcharm.Meta) error {
	defaults, err := s.st.StorageDefaults(ctx)
	if err != nil {
		return errors.Annotate(err, "getting storage defaults")
	}
	return domainstorage.StorageDirectivesWithDefaults(charmMeta.Storage, modelType, defaults, allDirectives)
}

func (s *ApplicationService) validateStorageDirectives(ctx context.Context, modelType coremodel.ModelType, allDirectives map[string]storage.Directive, charm internalcharm.Charm) error {
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

// UpdateCloudService updates the cloud service for the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
func (s *ApplicationService) UpdateCloudService(ctx context.Context, appName, providerID string, sAddrs network.SpaceAddresses) error {
	return s.st.UpsertCloudService(ctx, appName, providerID, sAddrs)
}

// Broker provides access to the k8s cluster to guery the scale
// of a specified application.
type Broker interface {
	Application(string, caas.DeploymentType) caas.Application
}

// CAASUnitTerminating should be called by the CAASUnitTerminationWorker when
// the agent receives a signal to exit. UnitTerminating will return how
// the agent should shutdown.
// We pass in a CAAS broker to get app details from the k8s cluster - we will probably
// make it a service attribute once more use cases emerge.
func (s *ApplicationService) CAASUnitTerminating(ctx context.Context, appName string, unitNum int, broker Broker) (bool, error) {
	// TODO(sidecar): handle deployment other than statefulset
	deploymentType := caas.DeploymentStateful
	restart := true

	switch deploymentType {
	case caas.DeploymentStateful:
		caasApp := broker.Application(appName, caas.DeploymentStateful)
		appState, err := caasApp.State()
		if err != nil {
			return false, errors.Trace(err)
		}
		var scaleInfo application.ScaleState
		err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
			appID, err := s.st.GetApplicationID(ctx, appName)
			if err != nil {
				return errors.Trace(err)
			}
			scaleInfo, err = s.st.GetApplicationScaleState(ctx, appID)
			return errors.Trace(err)
		})
		if err != nil {
			return false, errors.Trace(err)
		}
		if unitNum >= scaleInfo.Scale || unitNum >= appState.DesiredReplicas {
			restart = false
		}
	case caas.DeploymentStateless, caas.DeploymentDaemon:
		// Both handled the same way.
		restart = true
	default:
		return false, errors.NotSupportedf("unknown deployment type")
	}
	return restart, nil
}

// GetApplicationLife looks up the life of the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application is not found.
func (s *ApplicationService) GetApplicationLife(ctx context.Context, appName string) (corelife.Value, error) {
	var result corelife.Value
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		_, appLife, err := s.st.GetApplicationLife(ctx, appName)
		result = appLife.Value()
		return errors.Annotatef(err, "getting life for %q", appName)
	})
	return result, errors.Trace(err)
}

// SetApplicationScale sets the application's desired scale value, returning an error
// satisfying [applicationerrors.ApplicationNotFound] if the application is not found.
// This is used on CAAS models.
func (s *ApplicationService) SetApplicationScale(ctx context.Context, appName string, scale int) error {
	if scale < 0 {
		return fmt.Errorf("application scale %d not valid%w", scale, errors.Hide(applicationerrors.ScaleChangeInvalid))
	}
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if err != nil {
			return errors.Trace(err)
		}
		appScale, err := s.st.GetApplicationScaleState(ctx, appID)
		if err != nil {
			return errors.Annotatef(err, "getting application scale state for app %q", appID)
		}
		s.logger.Tracef(
			"SetScale DesiredScale %v -> %v", appScale.Scale, scale,
		)
		return s.st.SetDesiredApplicationScale(ctx, appID, scale)
	})
	return errors.Annotatef(err, "setting scale for application %q", appName)
}

// GetApplicationScale returns the desired scale of an application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// This is used on CAAS models.
func (s *ApplicationService) GetApplicationScale(ctx context.Context, appName string) (int, error) {
	_, scale, err := s.getApplicationScaleAndID(ctx, appName)
	return scale, errors.Trace(err)
}

func (s *ApplicationService) getApplicationScaleAndID(ctx context.Context, appName string) (coreapplication.ID, int, error) {
	var (
		scaleState application.ScaleState
		appID      coreapplication.ID
	)
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		appID, err = s.st.GetApplicationID(ctx, appName)
		if err != nil {
			return errors.Trace(err)
		}

		scaleState, err = s.st.GetApplicationScaleState(ctx, appID)
		return errors.Annotatef(err, "getting scaling state for %q", appName)
	})
	return appID, scaleState.Scale, errors.Trace(err)
}

// ChangeApplicationScale alters the existing scale by the provided change amount, returning the new amount.
// It returns an error satisfying [applicationerrors.ApplicationNotFoundError] if the application
// doesn't exist.
// This is used on CAAS models.
func (s *ApplicationService) ChangeApplicationScale(ctx context.Context, appName string, scaleChange int) (int, error) {
	var newScale int
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if err != nil {
			return errors.Trace(err)
		}
		currentScaleState, err := s.st.GetApplicationScaleState(ctx, appID)
		if err != nil {
			return errors.Annotatef(err, "getting current scale state for %q", appName)
		}

		newScale = currentScaleState.Scale + scaleChange
		s.logger.Tracef("ChangeScale DesiredScale %v, scaleChange %v, newScale %v", currentScaleState.Scale, scaleChange, newScale)
		if newScale < 0 {
			newScale = currentScaleState.Scale
			return fmt.Errorf(
				"%w: cannot remove more units than currently exist", applicationerrors.ScaleChangeInvalid)
		}
		err = s.st.SetDesiredApplicationScale(ctx, appID, newScale)
		return errors.Annotatef(err, "changing scaling state for %q", appName)
	})
	return newScale, errors.Annotatef(err, "changing scale for %q", appName)
}

// SetApplicationScalingState updates the scale state of an application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// This is used on CAAS models.
func (s *ApplicationService) SetApplicationScalingState(ctx context.Context, appName string, scaleTarget int, scaling bool) error {
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, appLife, err := s.st.GetApplicationLife(ctx, appName)
		if err != nil {
			return errors.Annotatef(err, "getting life for %q", appName)
		}
		currentScaleState, err := s.st.GetApplicationScaleState(ctx, appID)
		if err != nil {
			return errors.Annotatef(err, "getting current scale state for %q", appName)
		}

		var scale *int
		if scaling {
			switch appLife {
			case life.Alive:
				// if starting a scale, ensure we are scaling to the same target.
				if !currentScaleState.Scaling && currentScaleState.Scale != scaleTarget {
					return applicationerrors.ScalingStateInconsistent
				}
			case life.Dying, life.Dead:
				// force scale to the scale target when dying/dead.
				scale = &scaleTarget
			}
		}
		err = s.st.SetApplicationScalingState(ctx, appID, scale, scaleTarget, scaling)
		return errors.Annotatef(err, "updating scaling state for %q", appName)
	})
	return errors.Annotatef(err, "setting scale for %q", appName)

}

// GetApplicationScalingState returns the scale state of an application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// This is used on CAAS models.
func (s *ApplicationService) GetApplicationScalingState(ctx context.Context, appName string) (ScalingState, error) {
	var scaleState application.ScaleState
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if err != nil {
			return errors.Trace(err)
		}
		scaleState, err = s.st.GetApplicationScaleState(ctx, appID)
		return errors.Annotatef(err, "getting scaling state for %q", appName)
	})
	return ScalingState{
		ScaleTarget: scaleState.ScaleTarget,
		Scaling:     scaleState.Scaling,
	}, errors.Trace(err)
}

// AgentVersionGetter is responsible for retrieving the agent version for a
// given model.
type AgentVersionGetter interface {
	// GetModelAgentVersion returns the agent version for the specified model.
	GetModelAgentVersion(context.Context, coremodel.UUID) (version.Number, error)
}

// Provider defines the interface for interacting with the underlying model
// provider.
type Provider interface {
	environs.SupportedFeatureEnumerator
}

// ProviderApplicationService defines a service for interacting with the underlying
// model state.
type ProviderApplicationService struct {
	ApplicationService

	modelID            coremodel.UUID
	agentVersionGetter AgentVersionGetter
	provider           providertracker.ProviderGetter[Provider]
}

// NewProviderApplicationService returns a new Service for interacting with a models state.
func NewProviderApplicationService(
	st ApplicationState, params ApplicationServiceParams, logger logger.Logger,
	modelID coremodel.UUID,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
) *ProviderApplicationService {
	service := NewApplicationService(st, params, logger)

	return &ProviderApplicationService{
		ApplicationService: *service,
		modelID:            modelID,
		agentVersionGetter: agentVersionGetter,
		provider:           provider,
	}
}

// GetSupportedFeatures returns the set of features that the model makes
// available for charms to use.
// If the agent version cannot be found, an error satisfying
// [modelerrors.NotFound] will be returned.
func (s *ProviderApplicationService) GetSupportedFeatures(ctx context.Context) (assumes.FeatureSet, error) {
	agentVersion, err := s.agentVersionGetter.GetModelAgentVersion(ctx, s.modelID)
	if err != nil {
		return assumes.FeatureSet{}, err
	}

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{
		Name:        "juju",
		Description: assumes.UserFriendlyFeatureDescriptions["juju"],
		Version:     &agentVersion,
	})

	provider, err := s.provider(ctx)
	if errors.Is(err, errors.NotSupported) {
		return fs, nil
	} else if err != nil {
		return fs, err
	}

	envFs, err := provider.SupportedFeatures()
	if err != nil {
		return fs, fmt.Errorf("enumerating features supported by environment: %w", err)
	}

	fs.Merge(envFs)

	return fs, nil
}

// WatchableApplicationService provides the API for working with applications and the
// ability to create watchers.
type WatchableApplicationService struct {
	ProviderApplicationService
	watcherFactory WatcherFactory
}

// NewWatchableApplicationService returns a new service reference wrapping the input state.
func NewWatchableApplicationService(
	st ApplicationState, watcherFactory WatcherFactory,
	params ApplicationServiceParams,
	logger logger.Logger,
	modelID coremodel.UUID,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
) *WatchableApplicationService {
	service := NewProviderApplicationService(st, params, logger, modelID, agentVersionGetter, provider)

	return &WatchableApplicationService{
		ProviderApplicationService: *service,
		watcherFactory:             watcherFactory,
	}
}

// WatchApplicationUnitLife returns a watcher that observes changes to the life of any units if an application.
func (s *WatchableApplicationService) WatchApplicationUnitLife(appName string) (watcher.StringsWatcher, error) {
	lifeGetter := func(ctx context.Context, db coredatabase.TxnRunner, ids []string) (map[string]life.Life, error) {
		return s.st.GetApplicationUnitLife(ctx, appName, ids...)
	}
	lifeMapper := domain.LifeStringsWatcherMapperFunc(s.logger, lifeGetter)

	table, query := s.st.InitialWatchStatementUnitLife(appName)
	return s.watcherFactory.NewNamespaceMapperWatcher(table, changestream.All, query, lifeMapper)
}

// WatchApplicationScale returns a watcher that observes changes to an application's scale.
func (s *WatchableApplicationService) WatchApplicationScale(ctx context.Context, appName string) (watcher.NotifyWatcher, error) {
	appID, currentScale, err := s.getApplicationScaleAndID(ctx, appName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	mask := changestream.Create | changestream.Update
	mapper := func(ctx context.Context, db coredatabase.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
		newScale, err := s.GetApplicationScale(ctx, appName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// Only dispatch if the scale has changed.
		if newScale != currentScale {
			currentScale = newScale
			return changes, nil
		}
		return nil, nil
	}
	return s.watcherFactory.NewValueMapperWatcher("application_scale", appID.String(), mask, mapper)
}

// isValidApplicationName returns whether name is a valid application name.
func isValidApplicationName(name string) bool {
	return validApplication.MatchString(name)
}

// isValidReferenceName returns whether name is a valid reference name.
// This ensures that the reference name is both a valid application name
// and a valid charm name.
func isValidReferenceName(name string) bool {
	return isValidApplicationName(name) && isValidCharmName(name)
}
