// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/names/v5"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/leadership"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coresecrets "github.com/juju/juju/core/secrets"
	corestatus "github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/linklayerdevice"
	domainstorage "github.com/juju/juju/domain/storage"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
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
	// satisfying [applicationerrors.ApplicationNotFoundError] if the
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
}

// ApplicationState describes retrieval and persistence methods for
// applications.
type ApplicationState interface {
	AtomicApplicationState

	// GetModelType returns the model type for the underlying model. If the model
	// does not exist then an error satisfying [modelerrors.NotFound] will be returned.
	GetModelType(context.Context) (coremodel.ModelType, error)

	// StorageDefaults returns the default storage sources for a model.
	StorageDefaults(context.Context) (domainstorage.StorageDefaults, error)

	// GetStoragePoolByName returns the storage pool with the specified name,
	// returning an error satisfying [storageerrors.PoolNotFoundError] if it
	// doesn't exist.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePoolDetails, error)

	// GetUnitUUIDs returns the UUIDs for the named units in bulk, returning an
	// error satisfying [applicationerrors.UnitNotFound] if any of the units don't
	// exist.
	GetUnitUUIDs(context.Context, []coreunit.Name) ([]coreunit.UUID, error)

	// GetUnitNames gets in bulk the names for the specified unit UUIDs, returning
	// an error satisfying [applicationerrors.UnitNotFound] if any units are not
	// found.
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
	// [applicationerrors.ApplicationNotFoundError] is returned.
	// If the charm for the application does not exist, an error satisfying
	// [applicationerrors.CharmNotFoundError] is returned.
	GetCharmByApplicationID(context.Context, coreapplication.ID) (domaincharm.Charm, error)

	// GetCharmIDByApplicationName returns a charm ID by application name. It
	// returns an error if the charm can not be found by the name. This can also
	// be used as a cheap way to see if a charm exists without needing to load
	// the charm metadata.
	GetCharmIDByApplicationName(context.Context, string) (corecharm.ID, error)

	// GetApplicationIDByUnitName returns the application ID for the named unit,
	// returning an error satisfying [applicationerrors.UnitNotFound] if the unit
	// doesn't exist.
	GetApplicationIDByUnitName(ctx context.Context, name coreunit.Name) (coreapplication.ID, error)

	// GetCharmModifiedVersion looks up the charm modified version of the given
	// application. Returns [applicationerrors.ApplicationNotFound] if the
	// application is not found.
	GetCharmModifiedVersion(ctx context.Context, id coreapplication.ID) (int, error)

	// GetApplicationsWithPendingCharmsFromUUIDs returns the applications
	// with pending charms for the specified UUIDs. If the application has a
	// different status, it's ignored.
	GetApplicationsWithPendingCharmsFromUUIDs(ctx context.Context, uuids []coreapplication.ID) ([]coreapplication.ID, error)
}

// DeleteSecretState describes methods used by the secret deleter plugin.
type DeleteSecretState interface {
	// DeleteSecret deletes the specified secret revisions.
	// If revisions is nil the last remaining revisions are removed.
	DeleteSecret(ctx domain.AtomicContext, uri *coresecrets.URI, revs []int) error
}

// CreateApplication creates the specified application and units if required,
// returning an error satisfying [applicationerrors.ApplicationAlreadyExists]
// if the application already exists.
func (s *Service) CreateApplication(
	ctx context.Context,
	name string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
	units ...AddUnitArg,
) (coreapplication.ID, error) {
	if err := validateCreateApplicationParams(name, args.ReferenceName, charm, origin, args.DownloadInfo); err != nil {
		return "", errors.Errorf("invalid application args: %w", err)
	}

	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return "", errors.Errorf("getting model type %w", err)
	}
	appArg, err := makeCreateApplicationArgs(ctx, s.st, s.storageRegistryGetter, modelType, charm, origin, args)
	if err != nil {
		return "", errors.Errorf("creating application args %w", err)
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
			return errors.Errorf("creating application %q %w", name, err)
		}
		return s.st.AddUnits(ctx, appID, unitArgs...)
	})
	return appID, err
}

func validateCreateApplicationParams(
	name, referenceName string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	downloadInfo *domaincharm.DownloadInfo,
) error {
	if !isValidApplicationName(name) {
		return applicationerrors.ApplicationNameNotValid
	}

	// We require a valid charm metadata.
	if meta := charm.Meta(); meta == nil {
		return applicationerrors.CharmMetadataNotValid
	} else if !isValidCharmName(meta.Name) {
		return applicationerrors.CharmNameNotValid
	}

	// We require a valid charm manifest.
	if manifest := charm.Manifest(); manifest == nil {
		return applicationerrors.CharmManifestNotFound
	} else if len(manifest.Bases) == 0 {
		return applicationerrors.CharmManifestNotValid
	}

	// If the reference name is provided, it must be valid.
	if !isValidReferenceName(referenceName) {
		return errors.Errorf("reference name: %w", applicationerrors.CharmNameNotValid)
	}

	// If the origin is from charmhub, then we require the download info.
	if origin.Source == corecharm.CharmHub {
		if downloadInfo == nil {
			return applicationerrors.CharmDownloadInfoNotFound
		}
		if err := downloadInfo.Validate(); err != nil {
			return errors.Errorf("download info: %w", err)
		}
	}

	// Validate the origin of the charm.
	if err := origin.Validate(); err != nil {
		return errors.Errorf("%w: %v", applicationerrors.CharmOriginNotValid, err)
	}
	return nil
}

func makeCreateApplicationArgs(
	ctx context.Context,
	state State,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
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

	meta := charm.Meta()

	var err error
	if cons, err = addDefaultStorageDirectives(ctx, state, modelType, cons, meta.Storage); err != nil {
		return application.AddApplicationArg{}, errors.Errorf("adding default storage directives %w", err)
	}
	if err := validateStorageDirectives(ctx, state, storageRegistryGetter, modelType, cons, meta); err != nil {
		return application.AddApplicationArg{}, errors.Errorf("invalid storage directives %w", err)
	}

	// When encoding the charm, this will also validate the charm metadata,
	// when parsing it.
	ch, _, err := encodeCharm(charm)
	if err != nil {
		return application.AddApplicationArg{}, errors.Errorf("encoding charm: %w", err)
	}

	revision := -1
	if origin.Revision != nil {
		revision = *origin.Revision
	}

	source, err := encodeCharmSource(origin.Source)
	if err != nil {
		return application.AddApplicationArg{}, errors.Errorf("encoding charm source: %w", err)
	}

	architecture := encodeArchitecture(origin.Platform.Architecture)
	if err != nil {
		return application.AddApplicationArg{}, errors.Errorf("encoding architecture: %w", err)
	}

	ch.Source = source
	ch.ReferenceName = args.ReferenceName
	ch.Revision = revision
	ch.Hash = origin.Hash
	ch.ArchivePath = args.CharmStoragePath
	ch.Available = args.CharmStoragePath != ""
	ch.Architecture = architecture

	channelArg, platformArg, err := encodeChannelAndPlatform(origin)
	if err != nil {
		return application.AddApplicationArg{}, errors.Errorf("encode charm origin: %w", err)
	}

	return application.AddApplicationArg{
		Charm:             ch,
		CharmDownloadInfo: args.DownloadInfo,
		Platform:          platformArg,
		Channel:           channelArg,
	}, nil
}

func (s *Service) addNewUnitStatusToArg(arg *application.UnitStatusArg, modelType coremodel.ModelType) {
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

// AddUnits adds the specified units to the application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
func (s *Service) AddUnits(ctx context.Context, name string, units ...AddUnitArg) error {
	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return errors.Errorf("getting model type %w", err)
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
			return errors.Capture(err)
		}
		return s.st.AddUnits(ctx, appID, args...)
	})
	return errors.Errorf("adding units to application %q %w", name, err)
}

// GetApplicationIDByUnitName returns the application ID for the named unit,
// returning an error satisfying [applicationerrors.UnitNotFound] if the unit
// doesn't exist.
func (s *Service) GetApplicationIDByUnitName(
	ctx context.Context,
	unitName coreunit.Name,
) (coreapplication.ID, error) {
	id, err := s.st.GetApplicationIDByUnitName(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting application id: %w", err)
	}
	return id, nil
}

// GetUnitUUIDs returns the UUIDs for the named units in bulk, returning an error
// satisfying [applicationerrors.UnitNotFound] if any of the units don't exist.
func (s *Service) GetUnitUUIDs(ctx context.Context, unitNames []coreunit.Name) ([]coreunit.UUID, error) {
	uuids, err := s.st.GetUnitUUIDs(ctx, unitNames)
	if err != nil {
		return nil, errors.Errorf("failed to get unit UUIDs: %w", err)
	}
	return uuids, nil
}

// GetUnitUUID returns the UUID for the named unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (s *Service) GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error) {
	uuids, err := s.GetUnitUUIDs(ctx, []coreunit.Name{unitName})
	if err != nil {
		return "", err
	}
	return uuids[0], nil
}

// GetUnitNames gets in bulk the names for the specified unit UUIDs, returning an
// error satisfying [applicationerrors.UnitNotFound] if any units are not found.
func (s *Service) GetUnitNames(ctx context.Context, unitUUIDs []coreunit.UUID) ([]coreunit.Name, error) {
	names, err := s.st.GetUnitNames(ctx, unitUUIDs)
	if err != nil {
		return nil, errors.Errorf("failed to get unit names: %w", err)
	}
	return names, nil
}

// GetUnitLife looks up the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFoundError] if the unit is not found.
func (s *Service) GetUnitLife(ctx context.Context, unitName coreunit.Name) (corelife.Value, error) {
	var result corelife.Value
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		unitLife, err := s.st.GetUnitLife(ctx, unitName)
		result = unitLife.Value()
		return errors.Errorf("getting life for %q %w", unitName, err)
	})
	return result, errors.Capture(err)
}

// DeleteUnit deletes the specified unit.
// TODO(units) - rework when dual write is refactored
// This method is called (mostly during cleanup) after a unit
// has been removed from mongo. The mongo calls are
// DestroyMaybeRemove, DestroyWithForce, RemoveWithForce.
func (s *Service) DeleteUnit(ctx context.Context, unitName coreunit.Name) error {
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return s.deleteUnit(ctx, unitName)
	})
	if err != nil {
		return errors.Errorf("deleting unit %q %w", unitName, err)
	}
	return nil
}

func (s *Service) deleteUnit(ctx domain.AtomicContext, unitName coreunit.Name) error {
	// Get unit owned secrets.
	uris, err := s.st.GetSecretsForUnit(ctx, unitName)
	if err != nil {
		return errors.Errorf("getting unit owned secrets for %q %w", unitName, err)
	}
	// Delete unit owned secrets.
	for _, uri := range uris {
		s.logger.Debugf("deleting unit %q secret: %s", unitName, uri.ID)
		err := s.secretDeleter.DeleteSecret(ctx, uri, nil)
		if err != nil {
			return errors.Errorf("deleting secret %q %w", uri, err)
		}
	}

	err = s.ensureUnitDead(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil
	}
	if err != nil {
		return errors.Capture(err)
	}

	isLast, err := s.st.DeleteUnit(ctx, unitName)
	if err != nil {
		return errors.Errorf("deleting unit %q %w", unitName, err)
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
func (s *Service) DestroyUnit(ctx context.Context, unitName coreunit.Name) error {
	// For now, all we do is advance the unit's life to Dying.
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return s.st.SetUnitLife(ctx, unitName, life.Dying)
	})
	return errors.Errorf("destroying unit %q %w", unitName, err)
}

// EnsureUnitDead is called by the unit agent just before it terminates.
// TODO(units): revisit his existing logic ported from mongo
// Note: the agent only calls this method once it gets notification
// that the unit has become dead, so there's strictly no need to call
// this method as the unit is already dead.
// This method is also called during cleanup from various cleanup jobs.
// If the unit is not found, an error satisfying [applicationerrors.UnitNotFound]
// is returned.
func (s *Service) EnsureUnitDead(ctx context.Context, unitName coreunit.Name, leadershipRevoker leadership.Revoker) error {
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
	return errors.Errorf("ensuring unit %q is dead %w", unitName, err)
}

func (s *Service) ensureUnitDead(ctx domain.AtomicContext, unitName coreunit.Name) (err error) {
	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}
	if unitLife == life.Dead {
		return nil
	}
	// TODO(units) - check for subordinates and storage attachments
	// For IAAS units, we need to do additional checks - these are still done in mongo.
	// If a unit still has subordinates, return applicationerrors.UnitHasSubordinates.
	// If a unit still has storage attachments, return applicationerrors.UnitHasStorageAttachments.
	err = s.st.SetUnitLife(ctx, unitName, life.Dead)
	return errors.Errorf("ensuring unit %q is dead %w", unitName, err)
}

// RemoveUnit is called by the deployer worker and caas application provisioner worker to
// remove from the model units which have transitioned to dead.
// TODO(units): revisit his existing logic ported from mongo
// Note: the callers of this method only do so after the unit has become dead, so
// there's strictly no need to call ensureUnitDead before removing.
// If the unit is still alive, an error satisfying [applicationerrors.UnitIsAlive]
// is returned. If the unit is not found, an error satisfying
// [applicationerrors.UnitNotFound] is returned.
func (s *Service) RemoveUnit(ctx context.Context, unitName coreunit.Name, leadershipRevoker leadership.Revoker) error {
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		unitLife, err := s.st.GetUnitLife(ctx, unitName)
		if err != nil {
			return errors.Capture(err)
		}
		if unitLife == life.Alive {
			return errors.Errorf("cannot remove unit %q: %w", unitName, applicationerrors.UnitIsAlive)
		}
		err = s.deleteUnit(ctx, unitName)
		return errors.Errorf("deleting unit %q %w", unitName, err)
	})
	if err != nil {
		return errors.Errorf("removing unit %q %w", unitName, err)
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
func (s *Service) RegisterCAASUnit(ctx context.Context, appName string, args RegisterCAASUnitParams) error {
	if args.PasswordHash == "" {
		return errors.Errorf("password hash %w", coreerrors.NotValid)
	}
	if args.ProviderId == "" {
		return errors.Errorf("provider id %w", coreerrors.NotValid)
	}
	if !args.OrderedScale {
		return errors.Errorf("registering CAAS units not supported without ordered unit IDs %w", coreerrors.NotImplemented)
	}
	if args.UnitName == "" {
		return errors.Errorf("missing unit name %w", coreerrors.NotValid)
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
			return errors.Capture(err)
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
			return errors.Errorf("dead unit %q already exists", args.UnitName).Add(applicationerrors.UnitAlreadyExists)
		}
		if err := s.st.UpdateUnitContainer(ctx, args.UnitName, cloudContainer); err != nil {
			return errors.Errorf("updating unit %q %w", args.UnitName, err)
		}

		// We want to transition to using unit UUID instead of name.
		unitUUID, err := s.st.GetUnitUUID(ctx, args.UnitName)
		if err != nil {
			return errors.Capture(err)
		}
		return s.st.SetUnitPassword(ctx, unitUUID, application.PasswordInfo{
			PasswordHash:  args.PasswordHash,
			HashAlgorithm: application.HashAlgorithmSHA256,
		})
	})
	return errors.Errorf("saving caas unit %q %w", args.UnitName, err)
}

func (s *Service) insertCAASUnit(
	ctx domain.AtomicContext, appID coreapplication.ID, orderedID int, arg application.InsertUnitArg,
) error {
	appScale, err := s.st.GetApplicationScaleState(ctx, appID)
	if err != nil {
		return errors.Errorf("getting application scale state for app %q %w", appID, err)
	}
	if orderedID >= appScale.Scale ||
		(appScale.Scaling && orderedID >= appScale.ScaleTarget) {
		return errors.Errorf("unrequired unit %s is not assigned", arg.UnitName).Add(applicationerrors.UnitNotAssigned)
	}
	return s.st.InsertUnit(ctx, appID, arg)
}

// UpdateCAASUnit updates the specified CAAS unit, returning an error
// satisfying applicationerrors.ApplicationNotAlive if the unit's
// application is not alive.
func (s *Service) UpdateCAASUnit(ctx context.Context, unitName coreunit.Name, params UpdateCAASUnitParams) error {
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
		return errors.Capture(err)
	}
	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		_, appLife, err := s.st.GetApplicationLife(ctx, appName)
		if err != nil {
			return errors.Errorf("getting application %q life: %w", appName, err)
		}
		if appLife != life.Alive {
			return errors.Errorf("application %q is not alive", appName).Add(applicationerrors.ApplicationNotAlive)
		}

		if cloudContainer != nil {
			if err := s.st.UpdateUnitContainer(ctx, unitName, cloudContainer); err != nil {
				return errors.Errorf("updating cloud container %q %w", unitName, err)
			}
		}
		// We want to transition to using unit UUID instead of name.
		unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
		if err != nil {
			return errors.Capture(err)
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
				return errors.Errorf("saving unit %q agent status  %w", unitName, err)
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
				return errors.Errorf("saving unit %q workload status  %w", unitName, err)
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
				return errors.Errorf("saving unit %q cloud container status  %w", unitName, err)
			}
		}
		return nil
	})
	return errors.Errorf("updating caas unit %q %w", unitName, err)
}

// SetUnitPassword updates the password for the specified unit, returning an error
// satisfying [applicationerrors.NotNotFound] if the unit doesn't exist.
func (s *Service) SetUnitPassword(ctx context.Context, unitName coreunit.Name, password string) error {
	return s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
		if err != nil {
			return errors.Capture(err)
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
func (s *Service) DeleteApplication(ctx context.Context, name string) error {
	var cleanups []func(context.Context)
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		cleanups, err = s.deleteApplication(ctx, name)
		return errors.Capture(err)
	})
	if err != nil {
		return errors.Errorf("deleting application %q %w", name, err)
	}
	for _, cleanup := range cleanups {
		cleanup(ctx)
	}
	return nil
}

func (s *Service) deleteApplication(ctx domain.AtomicContext, name string) ([]func(context.Context), error) {
	// Get app owned secrets.
	uris, err := s.st.GetSecretsForApplication(ctx, name)
	if err != nil {
		return nil, errors.Errorf("getting application owned secrets for %q %w", name, err)
	}
	// Delete app owned secrets.
	for _, uri := range uris {
		s.logger.Debugf("deleting application %q secret: %s", name, uri.ID)
		err := s.secretDeleter.DeleteSecret(ctx, uri, nil)
		if err != nil {
			return nil, errors.Errorf("deleting secret %q %w", uri, err)
		}
	}

	err = s.st.DeleteApplication(ctx, name)
	return nil, errors.Errorf("deleting application %q %w", name, err)
}

// DestroyApplication prepares an application for removal from the model
// returning an error  satisfying [applicationerrors.ApplicationNotFoundError]
// if the application doesn't exist.
func (s *Service) DestroyApplication(ctx context.Context, appName string) error {
	// For now, all we do is advance the application's life to Dying.
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			return nil
		}
		if err != nil {
			return errors.Capture(err)
		}
		return s.st.SetApplicationLife(ctx, appID, life.Dying)
	})
	return errors.Errorf("destroying application %q %w", appName, err)
}

// EnsureApplicationDead is called by the cleanup worker if a mongo
// destroy operation sets the application to dead.
// TODO(units): remove when everything is in dqlite.
func (s *Service) EnsureApplicationDead(ctx context.Context, appName string) error {
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			return nil
		}
		if err != nil {
			return errors.Capture(err)
		}
		return s.st.SetApplicationLife(ctx, appID, life.Dead)
	})
	return errors.Errorf("setting application %q life to Dead %w", appName, err)
}

// UpdateApplicationCharm sets a new charm for the application, validating that aspects such
// as storage are still viable with the new charm.
func (s *Service) UpdateApplicationCharm(ctx context.Context, name string, params UpdateCharmParams) error {
	//TODO(storage) - update charm and storage directive for app
	return nil
}

// GetApplicationIDByName returns a application ID by application name. It
// returns an error if the application can not be found by the name.
//
// Returns [applicationerrors.ApplicationNameNotValid] if the name is not valid,
// and [applicationerrors.ApplicationNotFound] if the application is not found.
func (s *Service) GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error) {
	if !isValidApplicationName(name) {
		return "", applicationerrors.ApplicationNameNotValid
	}

	var id coreapplication.ID
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, name)
		if err != nil {
			return errors.Capture(err)
		}
		id = appID
		return nil
	})
	return id, errors.Capture(err)
}

// GetCharmIDByApplicationName returns a charm ID by application name. It
// returns an error if the charm can not be found by the name. This can also be
// used as a cheap way to see if a charm exists without needing to load the
// charm metadata.
//
// Returns [applicationerrors.ApplicationNameNotValid] if the name is not valid,
// and [applicationerrors.CharmNotFound] if the charm is not found.
func (s *Service) GetCharmIDByApplicationName(ctx context.Context, name string) (corecharm.ID, error) {
	if !isValidApplicationName(name) {
		return "", applicationerrors.ApplicationNameNotValid
	}

	return s.st.GetCharmIDByApplicationName(ctx, name)
}

// GetCharmModifiedVersion looks up the charm modified version of the given
// application.
//
// Returns [applicationerrors.ApplicationNotFound] if the application is not found.
func (s *Service) GetCharmModifiedVersion(ctx context.Context, id coreapplication.ID) (int, error) {
	charmModifiedVersion, err := s.st.GetCharmModifiedVersion(ctx, id)
	if err != nil {
		return -1, errors.Errorf("getting the application charm modified version: %w", err)
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
func (s *Service) GetCharmByApplicationID(ctx context.Context, id coreapplication.ID) (
	internalcharm.Charm,
	domaincharm.CharmLocator,
	error,
) {
	if err := id.Validate(); err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Errorf("application ID: %w", err).Add(applicationerrors.ApplicationIDNotValid)
	}

	charm, err := s.st.GetCharmByApplicationID(ctx, id)
	if err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Capture(err)
	}

	// The charm needs to be decoded into the internalcharm.Charm type.

	metadata, err := decodeMetadata(charm.Metadata)
	if err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Capture(err)
	}

	manifest, err := decodeManifest(charm.Manifest)
	if err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Capture(err)
	}

	actions, err := decodeActions(charm.Actions)
	if err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Capture(err)
	}

	config, err := decodeConfig(charm.Config)
	if err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Capture(err)
	}

	lxdProfile, err := decodeLXDProfile(charm.LXDProfile)
	if err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Capture(err)
	}

	locator := domaincharm.CharmLocator{
		Name:         charm.ReferenceName,
		Revision:     charm.Revision,
		Source:       charm.Source,
		Architecture: charm.Architecture,
	}

	return internalcharm.NewCharmBase(
		&metadata,
		&manifest,
		&config,
		&actions,
		&lxdProfile,
	), locator, nil
}

// UpdateCloudService updates the cloud service for the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
func (s *Service) UpdateCloudService(ctx context.Context, appName, providerID string, sAddrs network.SpaceAddresses) error {
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
func (s *Service) CAASUnitTerminating(ctx context.Context, appName string, unitNum int, broker Broker) (bool, error) {
	// TODO(sidecar): handle deployment other than statefulset
	deploymentType := caas.DeploymentStateful
	restart := true

	switch deploymentType {
	case caas.DeploymentStateful:
		caasApp := broker.Application(appName, caas.DeploymentStateful)
		appState, err := caasApp.State()
		if err != nil {
			return false, errors.Capture(err)
		}
		var scaleInfo application.ScaleState
		err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
			appID, err := s.st.GetApplicationID(ctx, appName)
			if err != nil {
				return errors.Capture(err)
			}
			scaleInfo, err = s.st.GetApplicationScaleState(ctx, appID)
			return errors.Capture(err)
		})
		if err != nil {
			return false, errors.Capture(err)
		}
		if unitNum >= scaleInfo.Scale || unitNum >= appState.DesiredReplicas {
			restart = false
		}
	case caas.DeploymentStateless, caas.DeploymentDaemon:
		// Both handled the same way.
		restart = true
	default:
		return false, errors.Errorf("unknown deployment type %w", coreerrors.NotSupported)
	}
	return restart, nil
}

// GetApplicationLife looks up the life of the specified application, returning
// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
// application is not found.
func (s *Service) GetApplicationLife(ctx context.Context, appName string) (corelife.Value, error) {
	var result corelife.Value
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		_, appLife, err := s.st.GetApplicationLife(ctx, appName)
		result = appLife.Value()
		return errors.Errorf("getting life for %q %w", appName, err)
	})
	return result, errors.Capture(err)
}

// SetApplicationScale sets the application's desired scale value, returning an error
// satisfying [applicationerrors.ApplicationNotFound] if the application is not found.
// This is used on CAAS models.
func (s *Service) SetApplicationScale(ctx context.Context, appName string, scale int) error {
	if scale < 0 {
		return errors.Errorf("application scale %d not valid", scale).Add(applicationerrors.ScaleChangeInvalid)
	}
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if err != nil {
			return errors.Capture(err)
		}
		appScale, err := s.st.GetApplicationScaleState(ctx, appID)
		if err != nil {
			return errors.Errorf("getting application scale state for app %q %w", appID, err)
		}
		s.logger.Tracef(
			"SetScale DesiredScale %v -> %v", appScale.Scale, scale,
		)
		return s.st.SetDesiredApplicationScale(ctx, appID, scale)
	})
	return errors.Errorf("setting scale for application %q %w", appName, err)
}

// GetApplicationScale returns the desired scale of an application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// This is used on CAAS models.
func (s *Service) GetApplicationScale(ctx context.Context, appName string) (int, error) {
	_, scale, err := s.getApplicationScaleAndID(ctx, appName)
	return scale, errors.Capture(err)
}

func (s *Service) getApplicationScaleAndID(ctx context.Context, appName string) (coreapplication.ID, int, error) {
	var (
		scaleState application.ScaleState
		appID      coreapplication.ID
	)
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		appID, err = s.st.GetApplicationID(ctx, appName)
		if err != nil {
			return errors.Capture(err)
		}

		scaleState, err = s.st.GetApplicationScaleState(ctx, appID)
		return errors.Errorf("getting scaling state for %q %w", appName, err)
	})
	return appID, scaleState.Scale, errors.Capture(err)
}

// ChangeApplicationScale alters the existing scale by the provided change amount, returning the new amount.
// It returns an error satisfying [applicationerrors.ApplicationNotFoundError] if the application
// doesn't exist.
// This is used on CAAS models.
func (s *Service) ChangeApplicationScale(ctx context.Context, appName string, scaleChange int) (int, error) {
	var newScale int
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if err != nil {
			return errors.Capture(err)
		}
		currentScaleState, err := s.st.GetApplicationScaleState(ctx, appID)
		if err != nil {
			return errors.Errorf("getting current scale state for %q %w", appName, err)
		}

		newScale = currentScaleState.Scale + scaleChange
		s.logger.Tracef("ChangeScale DesiredScale %v, scaleChange %v, newScale %v", currentScaleState.Scale, scaleChange, newScale)
		if newScale < 0 {
			newScale = currentScaleState.Scale
			return errors.Errorf(
				"%w: cannot remove more units than currently exist", applicationerrors.ScaleChangeInvalid)

		}
		err = s.st.SetDesiredApplicationScale(ctx, appID, newScale)
		return errors.Errorf("changing scaling state for %q %w", appName, err)
	})
	return newScale, errors.Errorf("changing scale for %q %w", appName, err)
}

// SetApplicationScalingState updates the scale state of an application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// This is used on CAAS models.
func (s *Service) SetApplicationScalingState(ctx context.Context, appName string, scaleTarget int, scaling bool) error {
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, appLife, err := s.st.GetApplicationLife(ctx, appName)
		if err != nil {
			return errors.Errorf("getting life for %q %w", appName, err)
		}
		currentScaleState, err := s.st.GetApplicationScaleState(ctx, appID)
		if err != nil {
			return errors.Errorf("getting current scale state for %q %w", appName, err)
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
		return errors.Errorf("updating scaling state for %q %w", appName, err)
	})
	return errors.Errorf("setting scale for %q %w", appName, err)

}

// GetApplicationScalingState returns the scale state of an application,
// returning an error satisfying [applicationerrors.ApplicationNotFoundError] if
// the application doesn't exist. This is used on CAAS models.
func (s *Service) GetApplicationScalingState(ctx context.Context, appName string) (ScalingState, error) {
	var scaleState application.ScaleState
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appID, err := s.st.GetApplicationID(ctx, appName)
		if err != nil {
			return errors.Capture(err)
		}
		scaleState, err = s.st.GetApplicationScaleState(ctx, appID)
		return errors.Errorf("getting scaling state for %q %w", appName, err)
	})
	return ScalingState{
		ScaleTarget: scaleState.ScaleTarget,
		Scaling:     scaleState.Scaling,
	}, errors.Capture(err)
}

// GetApplicationsWithPendingCharmsFromUUIDs returns the application UUIDs that
// have pending charms from the provided UUIDs. If there are no applications
// with pending status charms, then those applications are ignored.
func (s *Service) GetApplicationsWithPendingCharmsFromUUIDs(ctx context.Context, uuids []coreapplication.ID) ([]coreapplication.ID, error) {
	if len(uuids) == 0 {
		return nil, nil
	}
	return s.st.GetApplicationsWithPendingCharmsFromUUIDs(ctx, uuids)
}

// ReserveCharmDownload reserves a charm download slot for the specified
// application. If the charm is already being downloaded, the method will
// return [applicationerrors.AlreadyDownloadingCharm]. The charm download
// information is returned which includes the charm name, origin and the
// digest.
func (s *Service) ReserveCharmDownload(ctx context.Context, appID coreapplication.ID) (application.CharmDownloadInfo, error) {
	if err := appID.Validate(); err != nil {
		return application.CharmDownloadInfo{}, errors.Errorf("application ID: %w", err)
	}

	return application.CharmDownloadInfo{}, errors.Errorf("ReserveCharmDownload %w", coreerrors.NotImplemented)
}

// ResolveCharmDownload resolves the charm download slot for the specified
// application. The method will update the charm with the specified charm
// information.
func (s *Service) ResolveCharmDownload(ctx context.Context, appID coreapplication.ID, resolve application.ResolveCharmDownload) error {
	if err := appID.Validate(); err != nil {
		return errors.Errorf("application ID: %w", err)
	}
	return errors.Errorf("ResolveCharmDownload %w", coreerrors.NotImplemented)
}
