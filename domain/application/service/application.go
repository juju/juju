// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
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
	corestatus "github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/linklayerdevice"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
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

	// GetUnitUUID returns the UUID for the named unit, returning an error
	// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
	GetUnitUUID(ctx domain.AtomicContext, unitName coreunit.Name) (coreunit.UUID, error)

	// CreateApplication creates an application, returning an error satisfying
	// [applicationerrors.ApplicationAlreadyExists] if the application already
	// exists. If returns as error satisfying [applicationerrors.CharmNotFound]
	// if the charm for the application is not found.
	CreateApplication(domain.AtomicContext, string, application.AddApplicationArg) (coreapplication.ID, error)

	// AddUnits adds the specified units to the application.
	AddUnits(domain.AtomicContext, coreapplication.ID, ...application.AddUnitArg) error

	// InsertUnit insert the specified application unit, returning an error
	// satisfying [applicationerrors.UnitAlreadyExists]
	// if the unit exists.
	InsertUnit(domain.AtomicContext, coreapplication.ID, application.InsertUnitArg) error

	// UpdateUnitContainer updates the cloud container for specified unit,
	// returning an error satisfying [applicationerrors.UnitNotFoundError]
	// if the unit doesn't exist.
	UpdateUnitContainer(domain.AtomicContext, coreunit.Name, *application.CloudContainer) error

	// SetUnitPassword updates the password for the specified unit UUID.
	SetUnitPassword(domain.AtomicContext, coreunit.UUID, application.PasswordInfo) error

	// SetCloudContainerStatus saves the given cloud container status,
	// overwriting any current status data. If returns an error satisfying
	// [applicationerrors.UnitNotFound] if the unit doesn't exist.
	SetCloudContainerStatus(domain.AtomicContext, coreunit.UUID, application.CloudContainerStatusStatusInfo) error

	// SetUnitAgentStatus saves the given unit agent status, overwriting any
	// current status data. If returns an error satisfying
	// [applicationerrors.UnitNotFound] if the unit doesn't exist.
	SetUnitAgentStatus(domain.AtomicContext, coreunit.UUID, application.UnitAgentStatusInfo) error

	// SetUnitWorkloadStatus saves the given unit workload status, overwriting
	// any current status data. If returns an error satisfying
	// [applicationerrors.UnitNotFound] if the unit doesn't exist.
	SetUnitWorkloadStatus(domain.AtomicContext, coreunit.UUID, application.UnitWorkloadStatusInfo) error

	// GetApplicationLife looks up the life of the specified application,
	// returning an error satisfying
	// [applicationerrors.ApplicationNotFoundError] if the application is not
	// found.
	GetApplicationLife(ctx domain.AtomicContext, appName string) (coreapplication.ID, life.Life, error)

	// SetApplicationLife sets the life of the specified application.
	SetApplicationLife(domain.AtomicContext, coreapplication.ID, life.Life) error

	// GetApplicationScaleState looks up the scale state of the specified
	// application, returning an error satisfying
	// [applicationerrors.ApplicationNotFound] if the application is not found.
	GetApplicationScaleState(domain.AtomicContext, coreapplication.ID) (application.ScaleState, error)

	// SetApplicationScalingState sets the scaling details for the given caas
	// application Scale is optional and is only set if not nil.
	SetApplicationScalingState(ctx domain.AtomicContext, appID coreapplication.ID, scale *int, targetScale int, scaling bool) error

	// SetDesiredApplicationScale updates the desired scale of the specified
	// application.
	SetDesiredApplicationScale(domain.AtomicContext, coreapplication.ID, int) error

	// GetUnitLife looks up the life of the specified unit, returning an error
	// satisfying [applicationerrors.UnitNotFound] if the unit is not found.
	GetUnitLife(domain.AtomicContext, coreunit.Name) (life.Life, error)

	// SetUnitLife sets the life of the specified unit.
	SetUnitLife(domain.AtomicContext, coreunit.Name, life.Life) error

	// InitialWatchStatementUnitLife returns the initial namespace query for the
	// application unit life watcher.
	InitialWatchStatementUnitLife(appName string) (string, eventsource.NamespaceQuery)

	// InitialWatchStatementApplicationsWithPendingCharms returns the initial
	// namespace query for the applications with pending charms watcher.
	InitialWatchStatementApplicationsWithPendingCharms() (string, eventsource.NamespaceQuery)

	// DeleteApplication deletes the specified application, returning an error
	// satisfying [applicationerrors.ApplicationNotFound] if the
	// application doesn't exist. If the application still has units, as error
	// satisfying [applicationerrors.ApplicationHasUnits] is returned.
	DeleteApplication(domain.AtomicContext, string) error

	// DeleteUnit deletes the specified unit.
	// If the unit's application is Dying and no
	// other references to it exist, true is returned to
	// indicate the application could be safely deleted.
	// It will fail if the unit is not Dead.
	DeleteUnit(domain.AtomicContext, coreunit.Name) (bool, error)

	// GetSecretsForUnit returns the secrets owned by the specified unit.
	GetSecretsForUnit(
		ctx domain.AtomicContext, unitName coreunit.Name,
	) ([]*coresecrets.URI, error)

	// GetSecretsForApplication returns the secrets owned by the specified
	// application.
	GetSecretsForApplication(
		ctx domain.AtomicContext, applicationName string,
	) ([]*coresecrets.URI, error)

	// GetApplicationsWithPendingCharmsFromUUIDs returns the applications
	// with pending charms for the specified UUIDs. If the application has a
	// different status, it's ignored.
	GetApplicationsWithPendingCharmsFromUUIDs(ctx context.Context, uuids []coreapplication.ID) ([]coreapplication.ID, error)

	// IsApplicationCharmAvailable returns whether the application charm is
	// available for the specified application ID. Returns
	// [applicationerrors.ApplicationNotFound] if the application is not found.
	IsApplicationCharmAvailable(ctx context.Context, appID coreapplication.ID) (bool, error)
}

// ApplicationState describes retrieval and persistence methods for
// applications.
type ApplicationState interface {
	AtomicApplicationState

	// GetModelType returns the model type for the underlying model. If the
	// model does not exist then an error satisfying [modelerrors.NotFound] will
	// be returned.
	GetModelType(context.Context) (coremodel.ModelType, error)

	// StorageDefaults returns the default storage sources for a model.
	StorageDefaults(context.Context) (domainstorage.StorageDefaults, error)

	// GetStoragePoolByName returns the storage pool with the specified name,
	// returning an error satisfying [storageerrors.PoolNotFoundError] if it
	// doesn't exist.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePoolDetails, error)

	// GetUnitUUIDs returns the UUIDs for the named units in bulk, returning an
	// error satisfying [applicationerrors.UnitNotFound] if any of the units
	// don't exist.
	GetUnitUUIDs(context.Context, []coreunit.Name) ([]coreunit.UUID, error)

	// GetUnitNames gets in bulk the names for the specified unit UUIDs,
	// returning an error satisfying [applicationerrors.UnitNotFound] if any
	// units are not found.
	GetUnitNames(context.Context, []coreunit.UUID) ([]coreunit.Name, error)

	// UpsertCloudService updates the cloud service for the specified
	// application, returning an error satisfying
	// [applicationerrors.ApplicationNotFoundError] if the application doesn't
	// exist.
	UpsertCloudService(ctx context.Context, appName, providerID string, sAddrs network.SpaceAddresses) error

	// GetApplicationUnitLife returns the life values for the specified units of
	// the given application. The supplied ids may belong to a different
	// application; the application name is used to filter.
	GetApplicationUnitLife(ctx context.Context, appName string, unitUUIDs ...coreunit.UUID) (map[coreunit.UUID]life.Life, error)

	// GetCharmByApplicationID returns the charm, charm origin and charm
	// platform for the specified application ID.
	//
	// If the application does not exist, an error satisfying
	// [applicationerrors.ApplicationNotFoundError] is returned. If the charm
	// for the application does not exist, an error satisfying
	// [applicationerrors.CharmNotFoundError] is returned.
	GetCharmByApplicationID(context.Context, coreapplication.ID) (domaincharm.Charm, domaincharm.CharmOrigin, application.Platform, error)

	// GetCharmInfoByApplicationID returns the charm info, which includes the
	// charm uuid, name and origin for the specified application ID.
	GetCharmInfoByApplicationID(context.Context, coreapplication.ID) (domaincharm.CharmInfo, error)

	// GetCharmIDByApplicationName returns a charm ID by application name. It
	// returns an error if the charm can not be found by the name. This can also
	// be used as a cheap way to see if a charm exists without needing to load
	// the charm metadata.
	GetCharmIDByApplicationName(context.Context, string) (corecharm.ID, error)

	// GetApplicationIDByUnitName returns the application ID for the named unit,
	// returning an error satisfying [applicationerrors.UnitNotFound] if the
	// unit doesn't exist.
	GetApplicationIDByUnitName(ctx context.Context, name coreunit.Name) (coreapplication.ID, error)

	// GetCharmModifiedVersion looks up the charm modified version of the given
	// application. Returns [applicationerrors.ApplicationNotFound] if the
	// application is not found.
	GetCharmModifiedVersion(ctx context.Context, id coreapplication.ID) (int, error)
}

// DeleteSecretState describes methods used by the secret deleter plugin.
type DeleteSecretState interface {
	// DeleteSecret deletes the specified secret revisions.
	// If revisions is nil the last remaining revisions are removed.
	DeleteSecret(ctx domain.AtomicContext, uri *coresecrets.URI, revs []int) error
}

const (
	// applicationSnippet is a non-compiled regexp that can be composed with
	// other snippets to form a valid application regexp.
	applicationSnippet = "(?:[a-z][a-z0-9]*(?:-[a-z0-9]*[a-z][a-z0-9]*)*)"
)

var (
	validApplication = regexp.MustCompile("^" + applicationSnippet + "$")
)

// ApplicationService provides the API for working with applications.
type ApplicationService struct {
	st     ApplicationState
	logger logger.Logger
	clock  clock.Clock

	charmDownloader       CharmDownloader
	charmStore            CharmStore
	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	secretDeleter         DeleteSecretState
}

// NewApplicationService returns a new service reference wrapping the input state.
func NewApplicationService(
	st ApplicationState,
	deleteSecretState DeleteSecretState,
	charmDownloader CharmDownloader,
	charmStore CharmStore,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	logger logger.Logger,
) *ApplicationService {
	return &ApplicationService{
		st:                    st,
		logger:                logger,
		clock:                 clock.WallClock,
		charmDownloader:       charmDownloader,
		charmStore:            charmStore,
		storageRegistryGetter: storageRegistryGetter,
		secretDeleter:         deleteSecretState,
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
	if err := s.validateCreateApplicationParams(name, args.ReferenceName, charm, origin); err != nil {
		return "", errors.Annotatef(err, "invalid application args")
	}

	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return "", errors.Annotatef(err, "getting model type")
	}
	appArg, err := s.makeCreateApplicationArgs(ctx, modelType, charm, origin, args)
	if err != nil {
		return "", errors.Annotatef(err, "creating application args")
	}
	// We know that the charm name is valid, so we can use it as the application
	// name if that is not provided.
	if name == "" {
		// Annoyingly this should be the reference name, but that's not
		// true in the previous code. To keep compatibility, we'll use the
		// charm name.
		name = appArg.Charm.Metadata.Name
	}

	appArg.Scale = len(units)

	unitArgs := make([]application.AddUnitArg, len(units))
	for i, u := range units {
		arg := application.AddUnitArg{
			UnitName: u.UnitName,
		}
		s.addNewUnitStatusToArg(&arg.UnitStatusArg, modelType)
		unitArgs[i] = arg
	}

	var appID coreapplication.ID
	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err = s.st.CreateApplication(ctx, name, appArg)
		if err != nil {
			return errors.Annotatef(err, "creating application %q", name)
		}
		return s.st.AddUnits(ctx, appID, unitArgs...)
	})
	return appID, err
}

func (s *ApplicationService) validateCreateApplicationParams(
	name, referenceName string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
) error {
	if !isValidApplicationName(name) {
		return applicationerrors.ApplicationNameNotValid
	}

	// Validate that we have a valid charm and name.
	meta := charm.Meta()
	if meta == nil {
		return applicationerrors.CharmMetadataNotValid
	}

	if !isValidCharmName(meta.Name) {
		return applicationerrors.CharmNameNotValid
	} else if !isValidReferenceName(referenceName) {
		return fmt.Errorf("reference name: %w", applicationerrors.CharmNameNotValid)
	}

	// Validate the origin of the charm.
	if err := origin.Validate(); err != nil {
		return fmt.Errorf("%w: %v", applicationerrors.CharmOriginNotValid, err)
	}
	return nil
}

func (s *ApplicationService) makeCreateApplicationArgs(
	ctx context.Context,
	modelType coremodel.ModelType,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
) (application.AddApplicationArg, error) {
	// TODO (stickupkid): These should be done either in the application
	// state in one transaction, or be operating on the domain/charm types.
	//TODO(storage) - insert storage directive for app

	cons := make(map[string]storage.Directive)
	for n, sc := range args.Storage {
		cons[n] = sc
	}
	if err := s.addDefaultStorageDirectives(ctx, modelType, cons, charm.Meta()); err != nil {
		return application.AddApplicationArg{}, errors.Annotate(err, "adding default storage directives")
	}
	if err := s.validateStorageDirectives(ctx, modelType, cons, charm); err != nil {
		return application.AddApplicationArg{}, errors.Annotate(err, "invalid storage directives")
	}

	// When encoding the charm, this will also validate the charm metadata,
	// when parsing it.
	ch, _, err := encodeCharm(charm)
	if err != nil {
		return application.AddApplicationArg{}, fmt.Errorf("encode charm: %w", err)
	}

	originArg, channelArg, platformArg, err := encodeCharmOrigin(origin, args.ReferenceName)
	if err != nil {
		return application.AddApplicationArg{}, fmt.Errorf("encode charm origin: %w", err)
	}

	return application.AddApplicationArg{
		Charm:            ch,
		Platform:         platformArg,
		Origin:           originArg,
		Channel:          channelArg,
		CharmStoragePath: args.CharmStoragePath,
	}, nil
}

func (s *ApplicationService) addNewUnitStatusToArg(arg *application.UnitStatusArg, modelType coremodel.ModelType) {
	now := s.clock.Now()
	arg.AgentStatus = application.UnitAgentStatusInfo{
		StatusID: application.UnitAgentStatusAllocating,
		StatusInfo: application.StatusInfo{
			Since: now,
		},
	}
	arg.WorkloadStatus = application.UnitWorkloadStatusInfo{
		StatusID: application.UnitWorkloadStatusWaiting,
		StatusInfo: application.StatusInfo{
			Message: corestatus.MessageInstallingAgent,
			Since:   now,
		},
	}
	if modelType == coremodel.IAAS {
		arg.WorkloadStatus.Message = corestatus.MessageWaitForMachine
	}
}

func (s *ApplicationService) makeUnitStatus(in StatusParams) application.StatusInfo {
	si := application.StatusInfo{
		Message: in.Message,
		Since:   s.clock.Now(),
	}
	if in.Since != nil {
		si.Since = *in.Since
	}
	if len(in.Data) > 0 {
		si.Data = make(map[string]string)
		for k, v := range in.Data {
			if v == nil {
				continue
			}
			si.Data[k] = fmt.Sprintf("%v", v)
		}
	}
	return si
}

// ImportApplication imports the specified application and units if required,
// returning an error satisfying [applicationerrors.ApplicationAlreadyExists]
// if the application already exists.
func (s *ApplicationService) ImportApplication(
	ctx context.Context, appName string,
	charm internalcharm.Charm, origin corecharm.Origin, args AddApplicationArgs,
	units ...ImportUnitArg,
) error {
	if err := s.validateCreateApplicationParams(appName, args.ReferenceName, charm, origin); err != nil {
		return errors.Annotatef(err, "invalid application args")
	}

	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return errors.Annotatef(err, "getting model type")
	}
	appArg, err := s.makeCreateApplicationArgs(ctx, modelType, charm, origin, args)
	if err != nil {
		return errors.Annotatef(err, "creating application args")
	}
	appArg.Scale = len(units)

	unitArgs := make([]application.InsertUnitArg, len(units))
	for i, u := range units {
		arg := application.InsertUnitArg{
			UnitName: u.UnitName,
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: application.UnitAgentStatusInfo{
					StatusID:   application.MarshallUnitAgentStatus(u.AgentStatus.Status),
					StatusInfo: s.makeUnitStatus(u.AgentStatus),
				},
				WorkloadStatus: application.UnitWorkloadStatusInfo{
					StatusID:   application.MarshallUnitWorkloadStatus(u.WorkloadStatus.Status),
					StatusInfo: s.makeUnitStatus(u.WorkloadStatus),
				},
			},
		}
		if u.CloudContainer != nil {
			arg.CloudContainer = makeCloudContainerArg(u.UnitName, *u.CloudContainer)
		}
		if u.PasswordHash != nil {
			arg.Password = &application.PasswordInfo{
				PasswordHash:  *u.PasswordHash,
				HashAlgorithm: application.HashAlgorithmSHA256,
			}
		}
		unitArgs[i] = arg
	}

	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.CreateApplication(ctx, appName, appArg)
		if err != nil {
			return errors.Annotatef(err, "creating application %q", appName)
		}
		for _, arg := range unitArgs {
			if err := s.st.InsertUnit(ctx, appID, arg); err != nil {
				return errors.Annotatef(err, "inserting unit %q", arg.UnitName)
			}
		}
		return nil
	})
	return err
}

// AddUnits adds the specified units to the application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
func (s *ApplicationService) AddUnits(ctx context.Context, name string, units ...AddUnitArg) error {
	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return errors.Annotatef(err, "getting model type")
	}

	args := make([]application.AddUnitArg, len(units))
	for i, u := range units {
		arg := application.AddUnitArg{
			UnitName: u.UnitName,
		}
		s.addNewUnitStatusToArg(&arg.UnitStatusArg, modelType)
		args[i] = arg
	}

	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, name)
		if err != nil {
			return errors.Trace(err)
		}
		return s.st.AddUnits(ctx, appID, args...)
	})
	return errors.Annotatef(err, "adding units to application %q", name)
}

// GetApplicationIDByUnitName returns the application ID for the named unit,
// returning an error satisfying [applicationerrors.UnitNotFound] if the unit
// doesn't exist.
func (s *ApplicationService) GetApplicationIDByUnitName(
	ctx context.Context,
	unitName coreunit.Name,
) (coreapplication.ID, error) {
	id, err := s.st.GetApplicationIDByUnitName(ctx, unitName)
	if err != nil {
		return "", internalerrors.Errorf("getting application id: %w", err)
	}
	return id, nil
}

// GetUnitUUIDs returns the UUIDs for the named units in bulk, returning an error
// satisfying [applicationerrors.UnitNotFound] if any of the units don't exist.
func (s *ApplicationService) GetUnitUUIDs(ctx context.Context, unitNames []coreunit.Name) ([]coreunit.UUID, error) {
	uuids, err := s.st.GetUnitUUIDs(ctx, unitNames)
	if err != nil {
		return nil, internalerrors.Errorf("failed to get unit UUIDs: %w", err)
	}
	return uuids, nil
}

// GetUnitUUID returns the UUID for the named unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (s *ApplicationService) GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error) {
	uuids, err := s.GetUnitUUIDs(ctx, []coreunit.Name{unitName})
	if err != nil {
		return "", err
	}
	return uuids[0], nil
}

// GetUnitNames gets in bulk the names for the specified unit UUIDs, returning an
// error satisfying [applicationerrors.UnitNotFound] if any units are not found.
func (s *ApplicationService) GetUnitNames(ctx context.Context, unitUUIDs []coreunit.UUID) ([]coreunit.Name, error) {
	names, err := s.st.GetUnitNames(ctx, unitUUIDs)
	if err != nil {
		return nil, internalerrors.Errorf("failed to get unit names: %w", err)
	}
	return names, nil
}

// GetUnitLife looks up the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFoundError] if the unit is not found.
func (s *ApplicationService) GetUnitLife(ctx context.Context, unitName coreunit.Name) (corelife.Value, error) {
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
func (s *ApplicationService) DeleteUnit(ctx context.Context, unitName coreunit.Name) error {
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return s.deleteUnit(ctx, unitName)
	})
	if err != nil {
		return errors.Annotatef(err, "deleting unit %q", unitName)
	}
	return nil
}

func (s *ApplicationService) deleteUnit(ctx domain.AtomicContext, unitName coreunit.Name) error {
	// Get unit owned secrets.
	uris, err := s.st.GetSecretsForUnit(ctx, unitName)
	if err != nil {
		return errors.Annotatef(err, "getting unit owned secrets for %q", unitName)
	}
	// Delete unit owned secrets.
	for _, uri := range uris {
		s.logger.Debugf("deleting unit %q secret: %s", unitName, uri.ID)
		err := s.secretDeleter.DeleteSecret(ctx, uri, nil)
		if err != nil {
			return errors.Annotatef(err, "deleting secret %q", uri)
		}
	}

	err = s.ensureUnitDead(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}

	isLast, err := s.st.DeleteUnit(ctx, unitName)
	if err != nil {
		return errors.Annotatef(err, "deleting unit %q", unitName)
	}
	if isLast {
		// TODO(units): schedule application cleanup
		_ = isLast
	}
	return nil
}

// DestroyUnit prepares a unit for removal from the model
// returning an error  satisfying [applicationerrors.UnitNotFoundError]
// if the unit doesn't exist.
func (s *ApplicationService) DestroyUnit(ctx context.Context, unitName coreunit.Name) error {
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
func (s *ApplicationService) EnsureUnitDead(ctx context.Context, unitName coreunit.Name, leadershipRevoker leadership.Revoker) error {
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return s.ensureUnitDead(ctx, unitName)
	})
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil
	}
	if err == nil {
		appName, _ := names.UnitApplication(unitName.String())
		if err := leadershipRevoker.RevokeLeadership(appName, unitName); err != nil && !errors.Is(err, leadership.ErrClaimNotHeld) {
			s.logger.Warningf("cannot revoke lease for dead unit %q", unitName)
		}
	}
	return errors.Annotatef(err, "ensuring unit %q is dead", unitName)
}

func (s *ApplicationService) ensureUnitDead(ctx domain.AtomicContext, unitName coreunit.Name) (err error) {
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
func (s *ApplicationService) RemoveUnit(ctx context.Context, unitName coreunit.Name, leadershipRevoker leadership.Revoker) error {
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		unitLife, err := s.st.GetUnitLife(ctx, unitName)
		if err != nil {
			return errors.Trace(err)
		}
		if unitLife == life.Alive {
			return fmt.Errorf("cannot remove unit %q: %w", unitName, applicationerrors.UnitIsAlive)
		}
		err = s.deleteUnit(ctx, unitName)
		return errors.Annotatef(err, "deleting unit %q", unitName)
	})
	if err != nil {
		return errors.Annotatef(err, "removing unit %q", unitName)
	}
	appName, _ := names.UnitApplication(unitName.String())
	if err := leadershipRevoker.RevokeLeadership(appName, unitName); err != nil && !errors.Is(err, leadership.ErrClaimNotHeld) {
		s.logger.Warningf("cannot revoke lease for dead unit %q", unitName)
	}
	return nil
}

func makeCloudContainerArg(unitName coreunit.Name, cloudContainer CloudContainerParams) *application.CloudContainer {
	result := &application.CloudContainer{
		ProviderId: cloudContainer.ProviderId,
		Ports:      cloudContainer.Ports,
	}
	if cloudContainer.Address != nil {
		// TODO(units) - handle the cloudContainer.Address space ID
		// For k8s we'll initially create a /32 subnet off the container address
		// and add that to the default space.
		result.Address = &application.ContainerAddress{
			// For cloud containers, the device is a placeholder without
			// a MAC address and once inserted, not updated. It just exists
			// to tie the address to the net node corresponding to the
			// cloud container.
			Device: application.ContainerDevice{
				Name:              fmt.Sprintf("placeholder for %q cloud container", unitName),
				DeviceTypeID:      linklayerdevice.DeviceTypeUnknown,
				VirtualPortTypeID: linklayerdevice.NonVirtualPortType,
			},
			Value:       cloudContainer.Address.Value,
			AddressType: ipaddress.MarshallAddressType(cloudContainer.Address.AddressType()),
			Scope:       ipaddress.MarshallScope(cloudContainer.Address.Scope),
			Origin:      ipaddress.MarshallOrigin(network.OriginProvider),
			ConfigType:  ipaddress.MarshallConfigType(network.ConfigDHCP),
		}
		if cloudContainer.AddressOrigin != nil {
			result.Address.Origin = ipaddress.MarshallOrigin(*cloudContainer.AddressOrigin)
		}
	}
	return result
}

// RegisterCAASUnit creates or updates the specified application unit in a caas model,
// returning an error satisfying [applicationerrors.ApplicationNotFoundError]
// if the application doesn't exist. If the unit life is Dead, an error
// satisfying [applicationerrors.UnitAlreadyExists] is returned.
func (s *ApplicationService) RegisterCAASUnit(ctx context.Context, appName string, args RegisterCAASUnitParams) error {
	if args.PasswordHash == "" {
		return errors.NotValidf("password hash")
	}
	if args.ProviderId == "" {
		return errors.NotValidf("provider id")
	}
	if !args.OrderedScale {
		return errors.NewNotImplemented(nil, "registering CAAS units not supported without ordered unit IDs")
	}
	if args.UnitName == "" {
		return errors.NotValidf("missing unit name")
	}

	cloudContainerParams := CloudContainerParams{
		ProviderId: args.ProviderId,
		Ports:      args.Ports,
	}
	if args.Address != nil {
		addr := network.NewSpaceAddress(*args.Address, network.WithScope(network.ScopeMachineLocal))
		cloudContainerParams.Address = &addr
		origin := network.OriginProvider
		cloudContainerParams.AddressOrigin = &origin
	}

	cloudContainer := makeCloudContainerArg(args.UnitName, cloudContainerParams)
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if err != nil {
			return errors.Trace(err)
		}
		unitLife, err := s.st.GetUnitLife(ctx, args.UnitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			arg := application.InsertUnitArg{
				UnitName: args.UnitName,
				Password: &application.PasswordInfo{
					PasswordHash:  args.PasswordHash,
					HashAlgorithm: application.HashAlgorithmSHA256,
				},
				CloudContainer: cloudContainer,
			}
			s.addNewUnitStatusToArg(&arg.UnitStatusArg, coremodel.CAAS)
			return s.insertCAASUnit(ctx, appID, args.OrderedId, arg)
		}
		if unitLife == life.Dead {
			return fmt.Errorf("dead unit %q already exists%w", args.UnitName, errors.Hide(applicationerrors.UnitAlreadyExists))
		}
		if err := s.st.UpdateUnitContainer(ctx, args.UnitName, cloudContainer); err != nil {
			return errors.Annotatef(err, "updating unit %q", args.UnitName)
		}

		// We want to transition to using unit UUID instead of name.
		unitUUID, err := s.st.GetUnitUUID(ctx, args.UnitName)
		if err != nil {
			return errors.Trace(err)
		}
		return s.st.SetUnitPassword(ctx, unitUUID, application.PasswordInfo{
			PasswordHash:  args.PasswordHash,
			HashAlgorithm: application.HashAlgorithmSHA256,
		})
	})
	return errors.Annotatef(err, "saving caas unit %q", args.UnitName)
}

func (s *ApplicationService) insertCAASUnit(
	ctx domain.AtomicContext, appID coreapplication.ID, orderedID int, arg application.InsertUnitArg,
) error {
	appScale, err := s.st.GetApplicationScaleState(ctx, appID)
	if err != nil {
		return errors.Annotatef(err, "getting application scale state for app %q", appID)
	}
	if orderedID >= appScale.Scale ||
		(appScale.Scaling && orderedID >= appScale.ScaleTarget) {
		return fmt.Errorf("unrequired unit %s is not assigned%w", arg.UnitName, errors.Hide(applicationerrors.UnitNotAssigned))
	}
	return s.st.InsertUnit(ctx, appID, arg)
}

// UpdateCAASUnit updates the specified CAAS unit, returning an error
// satisfying applicationerrors.ApplicationNotAlive if the unit's
// application is not alive.
func (s *ApplicationService) UpdateCAASUnit(ctx context.Context, unitName coreunit.Name, params UpdateCAASUnitParams) error {
	var cloudContainer *application.CloudContainer
	if params.ProviderId != nil {
		cloudContainerParams := CloudContainerParams{
			ProviderId: *params.ProviderId,
			Ports:      params.Ports,
		}
		if params.Address != nil {
			addr := network.NewSpaceAddress(*params.Address, network.WithScope(network.ScopeMachineLocal))
			cloudContainerParams.Address = &addr
			origin := network.OriginProvider
			cloudContainerParams.AddressOrigin = &origin
		}
		cloudContainer = makeCloudContainerArg(unitName, cloudContainerParams)
	}
	appName, err := names.UnitApplication(unitName.String())
	if err != nil {
		return errors.Trace(err)
	}
	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		_, appLife, err := s.st.GetApplicationLife(ctx, appName)
		if err != nil {
			return fmt.Errorf("getting application %q life: %w", appName, err)
		}
		if appLife != life.Alive {
			return fmt.Errorf("application %q is not alive%w", appName, errors.Hide(applicationerrors.ApplicationNotAlive))
		}

		if cloudContainer != nil {
			if err := s.st.UpdateUnitContainer(ctx, unitName, cloudContainer); err != nil {
				return errors.Annotatef(err, "updating cloud container %q", unitName)
			}
		}
		// We want to transition to using unit UUID instead of name.
		unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
		if err != nil {
			return errors.Trace(err)
		}
		now := time.Now()
		since := func(in *time.Time) time.Time {
			if in != nil {
				return *in
			}
			return now
		}
		if params.AgentStatus != nil {
			if err := s.st.SetUnitAgentStatus(ctx, unitUUID, application.UnitAgentStatusInfo{
				StatusID: application.MarshallUnitAgentStatus(params.AgentStatus.Status),
				StatusInfo: application.StatusInfo{
					Message: params.AgentStatus.Message,
					Data: transform.Map(
						params.AgentStatus.Data, func(k string, v any) (string, string) { return k, fmt.Sprint(v) }),
					Since: since(params.AgentStatus.Since),
				},
			}); err != nil {
				return errors.Annotatef(err, "saving unit %q agent status ", unitName)
			}
		}
		if params.WorkloadStatus != nil {
			if err := s.st.SetUnitWorkloadStatus(ctx, unitUUID, application.UnitWorkloadStatusInfo{
				StatusID: application.MarshallUnitWorkloadStatus(params.WorkloadStatus.Status),
				StatusInfo: application.StatusInfo{
					Message: params.WorkloadStatus.Message,
					Data: transform.Map(
						params.WorkloadStatus.Data, func(k string, v any) (string, string) { return k, fmt.Sprint(v) }),
					Since: since(params.WorkloadStatus.Since),
				},
			}); err != nil {
				return errors.Annotatef(err, "saving unit %q workload status ", unitName)
			}
		}
		if params.CloudContainerStatus != nil {
			if err := s.st.SetCloudContainerStatus(ctx, unitUUID, application.CloudContainerStatusStatusInfo{
				StatusID: application.MarshallCloudContainerStatus(params.CloudContainerStatus.Status),
				StatusInfo: application.StatusInfo{
					Message: params.CloudContainerStatus.Message,
					Data: transform.Map(
						params.CloudContainerStatus.Data, func(k string, v any) (string, string) { return k, fmt.Sprint(v) }),
					Since: since(params.CloudContainerStatus.Since),
				},
			}); err != nil {
				return errors.Annotatef(err, "saving unit %q cloud container status ", unitName)
			}
		}
		return nil
	})
	return errors.Annotatef(err, "updating caas unit %q", unitName)
}

// SetUnitPassword updates the password for the specified unit, returning an error
// satisfying [applicationerrors.NotNotFound] if the unit doesn't exist.
func (s *ApplicationService) SetUnitPassword(ctx context.Context, unitName coreunit.Name, password string) error {
	return s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
		if err != nil {
			return errors.Trace(err)
		}
		return s.st.SetUnitPassword(ctx, unitUUID, application.PasswordInfo{
			PasswordHash:  password,
			HashAlgorithm: application.HashAlgorithmSHA256,
		})
	})
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
	uris, err := s.st.GetSecretsForApplication(ctx, name)
	if err != nil {
		return nil, errors.Annotatef(err, "getting application owned secrets for %q", name)
	}
	// Delete app owned secrets.
	for _, uri := range uris {
		s.logger.Debugf("deleting application %q secret: %s", name, uri.ID)
		err := s.secretDeleter.DeleteSecret(ctx, uri, nil)
		if err != nil {
			return nil, errors.Annotatef(err, "deleting secret %q", uri)
		}
	}

	err = s.st.DeleteApplication(ctx, name)
	return nil, errors.Annotatef(err, "deleting application %q", name)
}

// DestroyApplication prepares an application for removal from the model
// returning an error  satisfying [applicationerrors.ApplicationNotFoundError]
// if the application doesn't exist.
func (s *ApplicationService) DestroyApplication(ctx context.Context, appName string) error {
	// For now, all we do is advance the application's life to Dying.
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
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
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
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

// GetApplicationIDByName returns a application ID by application name. It
// returns an error if the application can not be found by the name.
//
// Returns [applicationerrors.ApplicationNameNotValid] if the name is not valid,
// and [applicationerrors.ApplicationNotFound] if the application is not found.
func (s *ApplicationService) GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error) {
	if !isValidApplicationName(name) {
		return "", applicationerrors.ApplicationNameNotValid
	}

	var id coreapplication.ID
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, name)
		if err != nil {
			return errors.Trace(err)
		}
		id = appID
		return nil
	})
	return id, errors.Trace(err)
}

// GetCharmIDByApplicationName returns a charm ID by application name. It
// returns an error if the charm can not be found by the name. This can also be
// used as a cheap way to see if a charm exists without needing to load the
// charm metadata.
//
// Returns [applicationerrors.ApplicationNameNotValid] if the name is not valid,
// and [applicationerrors.CharmNotFound] if the charm is not found.
func (s *ApplicationService) GetCharmIDByApplicationName(ctx context.Context, name string) (corecharm.ID, error) {
	if !isValidApplicationName(name) {
		return "", applicationerrors.ApplicationNameNotValid
	}

	return s.st.GetCharmIDByApplicationName(ctx, name)
}

// GetCharmModifiedVersion looks up the charm modified version of the given
// application.
//
// Returns [applicationerrors.ApplicationNotFound] if the application is not found.
func (s *ApplicationService) GetCharmModifiedVersion(ctx context.Context, id coreapplication.ID) (int, error) {
	charmModifiedVersion, err := s.st.GetCharmModifiedVersion(ctx, id)
	if err != nil {
		return -1, internalerrors.Errorf("getting the application charm modified version: %w", err)
	}
	return charmModifiedVersion, nil
}

// GetCharmByApplicationID returns the charm for the specified application
// ID.
//
// If the application does not exist, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned. If the charm for the
// application does not exist, an error satisfying
// [applicationerrors.CharmNotFound is returned. If the application name is not
// valid, an error satisfying [applicationerrors.ApplicationNameNotValid] is
// returned.
func (s *ApplicationService) GetCharmByApplicationID(ctx context.Context, id coreapplication.ID) (
	internalcharm.Charm,
	domaincharm.CharmOrigin,
	application.Platform,
	error,
) {
	if err := id.Validate(); err != nil {
		return nil, domaincharm.CharmOrigin{}, application.Platform{}, internalerrors.Errorf("application ID: %w%w", err, errors.Hide(applicationerrors.ApplicationIDNotValid))
	}

	charm, origin, platform, err := s.st.GetCharmByApplicationID(ctx, id)
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
	registry, err := s.storageRegistryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	validator, err := domainstorage.NewStorageDirectivesValidator(modelType, registry, s.st)
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

// Broker provides access to the k8s cluster to query the scale
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

// GetApplicationLife looks up the life of the specified application, returning
// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
// application is not found.
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

// GetApplicationScalingState returns the scale state of an application,
// returning an error satisfying [applicationerrors.ApplicationNotFoundError] if
// the application doesn't exist. This is used on CAAS models.
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

// GetApplicationsWithPendingCharmsFromUUIDs returns the application UUIDs that
// have pending charms from the provided UUIDs. If there are no applications
// with pending status charms, then those applications are ignored.
func (s *ApplicationService) GetApplicationsWithPendingCharmsFromUUIDs(ctx context.Context, uuids []coreapplication.ID) ([]coreapplication.ID, error) {
	if len(uuids) == 0 {
		return nil, nil
	}

	// Validate all the UUIDs, before we start the transaction.
	for _, uuid := range uuids {
		if err := uuid.Validate(); err != nil {
			return nil, errors.Annotate(err, "application")
		}
	}

	return s.st.GetApplicationsWithPendingCharmsFromUUIDs(ctx, uuids)
}

// DownloadApplicationCharm downloads the charms for the specified
// applications. If the charm has already been downloaded, it will be skipped.
func (s *ApplicationService) DownloadApplicationCharm(ctx context.Context, uuid coreapplication.ID) error {
	if err := uuid.Validate(); err != nil {
		return errors.Annotate(err, "application")
	}

	// Although this isn't a fool proof way to ensure that we don't download
	// the identical charm twice, it's a good enough heuristic for now.
	if available, err := s.st.IsApplicationCharmAvailable(ctx, uuid); available {
		return nil
	} else if err != nil {
		return internalerrors.Errorf("checking if charm is available: %w", err)
	}

	info, err := s.st.GetCharmInfoByApplicationID(ctx, uuid)
	if err != nil {
		return internalerrors.Errorf("getting charm name and origin: %w", err)
	}

	// Download the charm, once it's been downloaded, we can then persist the
	// charm is the storage. The downloader guarantees that the archive is
	// cleaned up on error. Once it's successfully downloaded, then the clean
	// up of the temporary archive is now our responsibility,
	downloadResult, err := s.charmDownloader.Download(ctx, info.Name, info.Origin)
	if err != nil {
		return internalerrors.Errorf("downloading charm: %w", err)
	}

	// charmPath is the path to the charm archive that we've downloaded as
	// a temporary file.
	blobPath := downloadResult.Path

	defer func() {
		// If for any reason we can't remove the temporary archive, we log a
		// warning, so the operator can manually clean up the archive.
		err := os.Remove(blobPath)
		if err != nil {
			s.logger.Warningf("removing temporary downloaded charm archive: %v", err)
		}
	}()

	// Persist the charm in the charm store (objectstore) and return back the
	// object UUID of the location. We can use that UUID to look up the
	// location of the charm blob in the object store.
	persistedUUID, err := s.charmStore.Store(ctx, info.Name, blobPath, downloadResult.Size, downloadResult.Origin.Hash)
	if err != nil {
		return internalerrors.Errorf("persisting charm: %w", err)
	}

	// Only once we've persisted the charm, do we update the application with
	// the new charm.
	charm, err := charm.ReadCharmArchive(blobPath)
	if err != nil {
		return internalerrors.Errorf("reading charm archive: %w", err)
	}

	if err := s.st.ResolveDownloadedCharm(ctx, info.UUID, persistedUUID, charm); err != nil {
		return internalerrors.Errorf("resolving downloaded charm: %w", err)
	}

	return nil
}

// AgentVersionGetter is responsible for retrieving the agent version for a
// given model.
type AgentVersionGetter interface {
	// GetModelTargetAgentVersion returns the agent version for the specified
	// model.
	GetModelTargetAgentVersion(context.Context, coremodel.UUID) (version.Number, error)
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
	st ApplicationState, deleteSecretState DeleteSecretState,
	modelID coremodel.UUID,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
	charmDownloader CharmDownloader,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	logger logger.Logger,
) *ProviderApplicationService {
	service := NewApplicationService(st, deleteSecretState, charmDownloader, storageRegistryGetter, logger)

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
	agentVersion, err := s.agentVersionGetter.GetModelTargetAgentVersion(ctx, s.modelID)
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
	st ApplicationState, deleteSecretState DeleteSecretState,
	watcherFactory WatcherFactory,
	modelID coremodel.UUID,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	logger logger.Logger,
) *WatchableApplicationService {
	service := NewProviderApplicationService(st, deleteSecretState, modelID, agentVersionGetter, provider, storageRegistryGetter, logger)

	return &WatchableApplicationService{
		ProviderApplicationService: *service,
		watcherFactory:             watcherFactory,
	}
}

// WatchApplicationUnitLife returns a watcher that observes changes to the life of any units if an application.
func (s *WatchableApplicationService) WatchApplicationUnitLife(appName string) (watcher.StringsWatcher, error) {
	lifeGetter := func(ctx context.Context, db coredatabase.TxnRunner, ids []string) (map[string]life.Life, error) {
		unitUUIDs, err := transform.SliceOrErr(ids, coreunit.ParseID)
		if err != nil {
			return nil, err
		}
		lives, err := s.st.GetApplicationUnitLife(ctx, appName, unitUUIDs...)
		if err != nil {
			return nil, err
		}
		result := make(map[string]life.Life, len(lives))
		for unitUUID, life := range lives {
			result[unitUUID.String()] = life
		}
		return result, nil
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

// WatchApplicationsWithPendingCharms returns a watcher that observes changes to
// applications that have pending charms.
func (s *WatchableApplicationService) WatchApplicationsWithPendingCharms(ctx context.Context) (watcher.StringsWatcher, error) {

	table, query := s.st.InitialWatchStatementApplicationsWithPendingCharms()
	return s.watcherFactory.NewNamespaceMapperWatcher(
		table,
		changestream.Create,
		query,
		func(ctx context.Context, _ coredatabase.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
			return s.watchApplicationsWithPendingCharmsMapper(ctx, changes)
		},
	)
}

// watchApplicationsWithPendingCharmsMapper removes any applications that do not
// have pending charms.
func (s *WatchableApplicationService) watchApplicationsWithPendingCharmsMapper(ctx context.Context, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
	// Preserve the ordering of the changes, as this is a strings watcher
	// and we want to return the changes in the order they were received.

	appChanges := make(map[coreapplication.ID][]indexedChanged)
	uuids := make([]coreapplication.ID, 0)
	for i, change := range changes {
		appID, err := coreapplication.ParseID(change.Changed())
		if err != nil {
			return nil, err
		}

		if _, ok := appChanges[appID]; !ok {
			uuids = append(uuids, appID)
		}

		appChanges[appID] = append(appChanges[appID], indexedChanged{
			change: change,
			index:  i,
		})
	}

	// Get all the applications with pending charms using the uuids.
	apps, err := s.GetApplicationsWithPendingCharmsFromUUIDs(ctx, uuids)
	if err != nil {
		return nil, err
	}

	// If any applications have no pending charms, then return no changes.
	if len(apps) == 0 {
		return nil, nil
	}

	// Grab all the changes for the applications with pending charms,
	// ensuring they're indexed so we can sort them later.
	var indexed []indexedChanged
	for _, appID := range apps {
		events, ok := appChanges[appID]
		if !ok {
			s.logger.Errorf("application %q has pending charms but no change events", appID)
			continue
		}

		indexed = append(indexed, events...)
	}

	// Sort the index so they're preserved
	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].index < indexed[j].index
	})

	// Grab the changes in the order they were received.
	var results []changestream.ChangeEvent
	for _, result := range indexed {
		results = append(results, result.change)
	}

	return results, nil
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

type indexedChanged struct {
	change changestream.ChangeEvent
	index  int
}
