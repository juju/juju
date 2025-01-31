// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/leadership"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coresecrets "github.com/juju/juju/core/secrets"
	corestatus "github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/linklayerdevice"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	internalcharm "github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// AtomicApplicationState describes retrieval and persistence methods for
// applications that require atomic transactions.
// Deprecated: use ApplicationState instead.
type AtomicApplicationState interface {
	domain.AtomicStateBase

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
	SetUnitAgentStatusAtomic(domain.AtomicContext, coreunit.UUID, application.UnitAgentStatusInfo) error

	// SetUnitWorkloadStatus saves the given unit workload status, overwriting
	// any current status data. If returns an error satisfying
	// [applicationerrors.UnitNotFound] if the unit doesn't exist.
	SetUnitWorkloadStatusAtomic(domain.AtomicContext, coreunit.UUID, application.UnitWorkloadStatusInfo) error

	// GetApplicationLife looks up the life of the specified application,
	// returning an error satisfying
	// [applicationerrors.ApplicationNotFoundError] if the application is not
	// found.
	GetApplicationLife(ctx domain.AtomicContext, appName string) (coreapplication.ID, life.Life, error)

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

	// SetUnitLife sets the life of the specified unit.
	SetUnitLife(domain.AtomicContext, coreunit.Name, life.Life) error

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

	// GetApplicationIDByName returns the application ID for the named application.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// CreateApplication creates an application, returning an error satisfying
	// [applicationerrors.ApplicationAlreadyExists] if the application already
	// exists. If returns as error satisfying [applicationerrors.CharmNotFound]
	// if the charm for the application is not found.
	CreateApplication(context.Context, string, application.AddApplicationArg, []application.AddUnitArg) (coreapplication.ID, error)

	// AddUnits adds the specified units to the application.
	AddUnits(context.Context, coreapplication.ID, ...application.AddUnitArg) error

	// InsertCAASUnit inserts the specified CAAS application unit, returning an
	// error satisfying [applicationerrors.UnitAlreadyExists] if the unit exists.
	InsertCAASUnit(context.Context, coreapplication.ID, application.RegisterCAASUnitArg) error

	// InsertUnit insert the specified application unit, returning an error
	// satisfying [applicationerrors.UnitAlreadyExists]
	// if the unit exists.
	InsertUnit(context.Context, coreapplication.ID, application.InsertUnitArg) error

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

	// GetUnitUUIDByName returns the UUID for the named unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
	// exist.
	GetUnitUUIDByName(context.Context, coreunit.Name) (coreunit.UUID, error)

	// UpsertCloudService updates the cloud service for the specified
	// application, returning an error satisfying
	// [applicationerrors.ApplicationNotFoundError] if the application doesn't
	// exist.
	UpsertCloudService(ctx context.Context, appName, providerID string, sAddrs network.SpaceAddresses) error

	// GetApplicationUnitLife returns the life values for the specified units of
	// the given application. The supplied ids may belong to a different
	// application; the application name is used to filter.
	GetApplicationUnitLife(ctx context.Context, appName string, unitUUIDs ...coreunit.UUID) (map[coreunit.UUID]life.Life, error)

	// SetApplicationLife sets the life of the specified application.
	SetApplicationLife(context.Context, coreapplication.ID, life.Life) error

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
	// returning an error satisfying [applicationerrors.UnitNotFound] if the
	// unit doesn't exist.
	GetApplicationIDByUnitName(ctx context.Context, name coreunit.Name) (coreapplication.ID, error)

	// GetCharmModifiedVersion looks up the charm modified version of the given
	// application. Returns [applicationerrors.ApplicationNotFound] if the
	// application is not found.
	GetCharmModifiedVersion(ctx context.Context, id coreapplication.ID) (int, error)

	// GetApplicationsWithPendingCharmsFromUUIDs returns the applications
	// with pending charms for the specified UUIDs. If the application has a
	// different status, it's ignored.
	GetApplicationsWithPendingCharmsFromUUIDs(ctx context.Context, uuids []coreapplication.ID) ([]coreapplication.ID, error)

	// GetAsyncCharmDownloadInfo reserves the charm download for the specified
	// application, returning an error satisfying
	// [applicationerrors.AlreadyDownloadingCharm] if the application is already
	// downloading a charm.
	GetAsyncCharmDownloadInfo(ctx context.Context, appID coreapplication.ID) (application.CharmDownloadInfo, error)

	// ResolveCharmDownload resolves the charm download for the specified
	// application, updating the charm with the specified charm information.
	ResolveCharmDownload(ctx context.Context, charmID corecharm.ID, info application.ResolvedCharmDownload) error

	// GetApplicationsForRevisionUpdater returns all the applications for the
	// revision updater. This will only return charmhub charms, for applications
	// that are alive.
	// This will return an empty slice if there are no applications.
	GetApplicationsForRevisionUpdater(ctx context.Context) ([]application.RevisionUpdaterApplication, error)

	// GetCharmConfigByApplicationID returns the charm config for the specified
	// application ID.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	// If the charm for the application does not exist, an error satisfying
	// [applicationerrors.CharmNotFoundError] is returned.
	GetCharmConfigByApplicationID(ctx context.Context, appID coreapplication.ID) (corecharm.ID, charm.Config, error)

	// GetApplicationConfigAndSettings returns the application config and
	// settings attributes for the application ID.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationConfigAndSettings(ctx context.Context, appID coreapplication.ID) (
		map[string]application.ApplicationConfig,
		application.ApplicationSettings,
		error,
	)

	// GetApplicationTrustSetting returns the application trust setting.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationTrustSetting(ctx context.Context, appID coreapplication.ID) (bool, error)

	// SetApplicationConfigAndSettings sets the application config attributes
	// using the configuration, and sets the trust setting as part of the
	// application.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	SetApplicationConfigAndSettings(
		ctx context.Context,
		appID coreapplication.ID,
		charmID corecharm.ID,
		config map[string]application.ApplicationConfig,
		settings application.ApplicationSettings,
	) error

	// UnsetApplicationConfigKeys removes the specified keys from the application
	// config. If the key does not exist, it is ignored.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	UnsetApplicationConfigKeys(ctx context.Context, appID coreapplication.ID, keys []string) error

	// GetUnitLife looks up the life of the specified unit, returning an error
	// satisfying [applicationerrors.UnitNotFound] if the unit is not found.
	GetUnitLife(context.Context, coreunit.Name) (life.Life, error)

	// InitialWatchStatementUnitLife returns the initial namespace query for the
	// application unit life watcher.
	InitialWatchStatementUnitLife(appName string) (string, eventsource.NamespaceQuery)

	// InitialWatchStatementApplicationsWithPendingCharms returns the initial
	// namespace query for the applications with pending charms watcher.
	InitialWatchStatementApplicationsWithPendingCharms() (string, eventsource.NamespaceQuery)
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
	if err := validateCreateApplicationParams(
		name,
		args.ReferenceName,
		charm,
		origin,
		args.DownloadInfo,
		args.ResolvedResources,
		s.logger,
	); err != nil {
		return "", errors.Annotatef(err, "invalid application args")
	}

	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return "", errors.Annotatef(err, "getting model type")
	}
	appArg, err := makeCreateApplicationArgs(ctx, s.st, s.storageRegistryGetter, modelType, charm, origin, args)
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

	numUnits := len(units)
	appArg.Scale = numUnits

	unitArgs := make([]application.AddUnitArg, numUnits)
	for i, u := range units {
		arg := application.AddUnitArg{
			UnitName: u.UnitName,
		}
		s.addNewUnitStatusToArg(&arg.UnitStatusArg, modelType)
		unitArgs[i] = arg
	}

	appID, err := s.st.CreateApplication(ctx, name, appArg, unitArgs)
	if err != nil {
		return "", errors.Annotatef(err, "creating application %q", name)
	}
	return appID, nil
}

func validateCreateApplicationParams(
	name, referenceName string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	downloadInfo *domaincharm.DownloadInfo,
	resolvedResources ResolvedResources,
	logger logger.Logger,
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
		return fmt.Errorf("reference name: %w", applicationerrors.CharmNameNotValid)
	}

	// If the origin is from charmhub, then we require the download info.
	if origin.Source == corecharm.CharmHub {
		if downloadInfo == nil {
			return applicationerrors.CharmDownloadInfoNotFound
		}
		if err := downloadInfo.Validate(); err != nil {
			return fmt.Errorf("download info: %w", err)
		}
	}

	// Validate the origin of the charm.
	if err := origin.Validate(); err != nil {
		return fmt.Errorf("%w: %v", applicationerrors.CharmOriginNotValid, err)
	}

	// Validate consistency of resources origin and revision
	if err := resolvedResources.Validate(); err != nil {
		return err
	}

	// Validates that all charm resources are resolved
	appResourceSet := set.NewStrings()
	charmResourceSet := set.NewStrings()
	for _, res := range charm.Meta().Resources {
		charmResourceSet.Add(res.Name)
	}
	for _, res := range resolvedResources {
		appResourceSet.Add(res.Name)
	}
	unexpectedResources := appResourceSet.Difference(charmResourceSet)
	missingResources := charmResourceSet.Difference(appResourceSet)
	if !unexpectedResources.IsEmpty() {
		// This needs to be an error because it will cause a foreign constraint
		// failure on insert, which is less easy to understand.
		return internalerrors.Errorf("unexpected resources %v: %w", unexpectedResources.Values(),
			applicationerrors.InvalidResourceArgs)
	}
	if !missingResources.IsEmpty() {
		// Some resources are defined in the charm but not given when trying
		// to create the application.
		return internalerrors.Errorf("charm resources not resolved %v: %w", missingResources.Values(),
			applicationerrors.InvalidResourceArgs)
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
		return application.AddApplicationArg{}, errors.Annotate(err, "adding default storage directives")
	}
	if err := validateStorageDirectives(ctx, state, storageRegistryGetter, modelType, cons, meta); err != nil {
		return application.AddApplicationArg{}, errors.Annotate(err, "invalid storage directives")
	}

	// When encoding the charm, this will also validate the charm metadata,
	// when parsing it.
	ch, _, err := encodeCharm(charm)
	if err != nil {
		return application.AddApplicationArg{}, fmt.Errorf("encoding charm: %w", err)
	}

	revision := -1
	if origin.Revision != nil {
		revision = *origin.Revision
	}

	source, err := encodeCharmSource(origin.Source)
	if err != nil {
		return application.AddApplicationArg{}, fmt.Errorf("encoding charm source: %w", err)
	}

	architecture := encodeArchitecture(origin.Platform.Architecture)
	if err != nil {
		return application.AddApplicationArg{}, fmt.Errorf("encoding architecture: %w", err)
	}

	ch.Source = source
	ch.ReferenceName = args.ReferenceName
	ch.Revision = revision
	ch.Hash = origin.Hash
	ch.ArchivePath = args.CharmStoragePath
	ch.ObjectStoreUUID = args.CharmObjectStoreUUID
	ch.Architecture = architecture

	// If we have a storage path, then we know the charm is available.
	// This is passive for now, but once we update the application, the presence
	// of the object store UUID will be used to determine if the charm is
	// available.
	ch.Available = args.CharmStoragePath != ""

	channelArg, platformArg, err := encodeChannelAndPlatform(origin)
	if err != nil {
		return application.AddApplicationArg{}, fmt.Errorf("encoding charm origin: %w", err)
	}

	applicationConfig, err := encodeApplicationConfig(args.ApplicationConfig, ch.Config)
	if err != nil {
		return application.AddApplicationArg{}, fmt.Errorf("encoding application config: %w", err)
	}

	return application.AddApplicationArg{
		Charm:             ch,
		CharmDownloadInfo: args.DownloadInfo,
		Platform:          platformArg,
		Channel:           channelArg,
		Resources:         makeResourcesArgs(args.ResolvedResources),
		Config:            applicationConfig,
		Settings:          args.ApplicationSettings,
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
func (s *Service) AddUnits(ctx context.Context, appName string, units ...AddUnitArg) error {
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

	appUUID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return internalerrors.Errorf("getting application %q id: %w", appName, err)
	}

	err = s.st.AddUnits(ctx, appUUID, args...)
	if err != nil {
		return internalerrors.Errorf("adding units to application %q: %w", appName, err)
	}
	return nil
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
		return "", internalerrors.Errorf("getting application id: %w", err)
	}
	return id, nil
}

// GetUnitUUID returns the UUID for the named unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (s *Service) GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error) {
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return "", internalerrors.Errorf("getting UUID of unit %q: %w", unitName, err)
	}
	return unitUUID, nil
}

// GetUnitLife looks up the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFoundError] if the unit is not found.
func (s *Service) GetUnitLife(ctx context.Context, unitName coreunit.Name) (corelife.Value, error) {
	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return "", internalerrors.Errorf("getting life for %q: %w", unitName, err)
	}
	return unitLife.Value(), nil
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
		return errors.Annotatef(err, "deleting unit %q", unitName)
	}
	return nil
}

func (s *Service) deleteUnit(ctx domain.AtomicContext, unitName coreunit.Name) error {
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

	// TODO(units) - check for subordinates and storage attachments
	// For IAAS units, we need to do additional checks - these are still done in mongo.
	// If a unit still has subordinates, return applicationerrors.UnitHasSubordinates.
	// If a unit still has storage attachments, return applicationerrors.UnitHasStorageAttachments.
	err = s.st.SetUnitLife(ctx, unitName, life.Dead)
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
func (s *Service) DestroyUnit(ctx context.Context, unitName coreunit.Name) error {
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
func (s *Service) EnsureUnitDead(ctx context.Context, unitName coreunit.Name, leadershipRevoker leadership.Revoker) error {
	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return errors.Trace(err)
	}
	if unitLife == life.Dead {
		return nil
	}
	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		// TODO(units) - check for subordinates and storage attachments
		// For IAAS units, we need to do additional checks - these are still done in mongo.
		// If a unit still has subordinates, return applicationerrors.UnitHasSubordinates.
		// If a unit still has storage attachments, return applicationerrors.UnitHasStorageAttachments.
		err = s.st.SetUnitLife(ctx, unitName, life.Dead)
		return errors.Annotatef(err, "marking unit %q is dead", unitName)
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

// RemoveUnit is called by the deployer worker and caas application provisioner worker to
// remove from the model units which have transitioned to dead.
// TODO(units): revisit his existing logic ported from mongo
// Note: the callers of this method only do so after the unit has become dead, so
// there's strictly no need to set the life to Dead before removing.
// If the unit is still alive, an error satisfying [applicationerrors.UnitIsAlive]
// is returned. If the unit is not found, an error satisfying
// [applicationerrors.UnitNotFound] is returned.
func (s *Service) RemoveUnit(ctx context.Context, unitName coreunit.Name, leadershipRevoker leadership.Revoker) error {
	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return errors.Trace(err)
	}
	if unitLife == life.Alive {
		return fmt.Errorf("cannot remove unit %q: %w", unitName, applicationerrors.UnitIsAlive)
	}
	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
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

func makeCloudContainerArg(unitName coreunit.Name, cloudContainer application.CloudContainerParams) *application.CloudContainer {
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

// makeResourcesArgs creates a slice of AddApplicationResourceArg from ResolvedResources.
func makeResourcesArgs(resolvedResources ResolvedResources) []application.AddApplicationResourceArg {
	var result []application.AddApplicationResourceArg
	for _, res := range resolvedResources {
		result = append(result, application.AddApplicationResourceArg{
			Name:     res.Name,
			Revision: res.Revision,
			Origin:   res.Origin,
		})
	}
	return result
}

// RegisterCAASUnit creates or updates the specified application unit in a caas model,
// returning an error satisfying [applicationerrors.ApplicationNotFoundError]
// if the application doesn't exist. If the unit life is Dead, an error
// satisfying [applicationerrors.UnitAlreadyExists] is returned.
func (s *Service) RegisterCAASUnit(ctx context.Context, appName string, args application.RegisterCAASUnitArg) error {
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

	appUUID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return internalerrors.Errorf("getting application ID: %w", err)
	}
	err = s.st.InsertCAASUnit(ctx, appUUID, args)
	if err != nil {
		return internalerrors.Errorf("saving caas unit %q: %w", args.UnitName, err)
	}
	return nil
}

// UpdateCAASUnit updates the specified CAAS unit, returning an error
// satisfying applicationerrors.ApplicationNotAlive if the unit's
// application is not alive.
func (s *Service) UpdateCAASUnit(ctx context.Context, unitName coreunit.Name, params UpdateCAASUnitParams) error {
	var cloudContainer *application.CloudContainer
	if params.ProviderId != nil {
		cloudContainerParams := application.CloudContainerParams{
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
	// We want to transition to using unit UUID instead of name.
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
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
		now := time.Now()
		since := func(in *time.Time) time.Time {
			if in != nil {
				return *in
			}
			return now
		}
		if params.AgentStatus != nil {
			if err := s.st.SetUnitAgentStatusAtomic(ctx, unitUUID, application.UnitAgentStatusInfo{
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
			if err := s.st.SetUnitWorkloadStatusAtomic(ctx, unitUUID, application.UnitWorkloadStatusInfo{
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
func (s *Service) SetUnitPassword(ctx context.Context, unitName coreunit.Name, password string) error {
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Trace(err)
	}
	return s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
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

func (s *Service) deleteApplication(ctx domain.AtomicContext, name string) ([]func(context.Context), error) {
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
func (s *Service) DestroyApplication(ctx context.Context, appName string) error {
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil
	} else if err != nil {
		return internalerrors.Errorf("getting application ID: %w", err)
	}
	// For now, all we do is advance the application's life to Dying.
	err = s.st.SetApplicationLife(ctx, appID, life.Dying)
	if err != nil {
		return internalerrors.Errorf("destroying application %q: %w", appName, err)
	}
	return nil
}

// MarkApplicationDead is called by the cleanup worker if a mongo
// destroy operation sets the application to dead.
// TODO(units): remove when everything is in dqlite.
func (s *Service) MarkApplicationDead(ctx context.Context, appName string) error {
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return internalerrors.Errorf("getting application ID: %w", err)
	}
	err = s.st.SetApplicationLife(ctx, appID, life.Dead)
	if err != nil {
		return internalerrors.Errorf("setting application %q life to Dead: %w", appName, err)
	}
	return nil
}

// UpdateApplicationCharm sets a new charm for the application, validating that aspects such
// as storage are still viable with the new charm.
func (s *Service) UpdateApplicationCharm(ctx context.Context, name string, params UpdateCharmParams) error {
	//TODO(storage) - update charm and storage directive for app
	return nil
}

// GetApplicationIDByName returns an application ID by application name. It
// returns an error if the application can not be found by the name.
//
// Returns [applicationerrors.ApplicationNameNotValid] if the name is not valid,
// and [applicationerrors.ApplicationNotFound] if the application is not found.
func (s *Service) GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error) {
	if !isValidApplicationName(name) {
		return "", applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationIDByName(ctx, name)
	if err != nil {
		return "", errors.Trace(err)
	}
	return appID, nil
}

// GetCharmLocatorByApplicationName returns a CharmLocator by application name.
// It returns an error if the charm can not be found by the name. This can also
// be used as a cheap way to see if a charm exists without needing to load the
// charm metadata.
//
// Returns [applicationerrors.ApplicationNameNotValid] if the name is not valid,
// [applicationerrors.ApplicationNotFound] if the application is not found, and
// [applicationerrors.CharmNotFound] if the charm is not found.
func (s *Service) GetCharmLocatorByApplicationName(ctx context.Context, name string) (domaincharm.CharmLocator, error) {
	if !isValidApplicationName(name) {
		return domaincharm.CharmLocator{}, applicationerrors.ApplicationNameNotValid
	}

	charmID, err := s.st.GetCharmIDByApplicationName(ctx, name)
	if err != nil {
		return domaincharm.CharmLocator{}, errors.Trace(err)
	}

	locator, err := s.getCharmLocatorByID(ctx, charmID)
	return locator, errors.Trace(err)
}

// GetCharmModifiedVersion looks up the charm modified version of the given
// application.
//
// Returns [applicationerrors.ApplicationNotFound] if the application is not found.
func (s *Service) GetCharmModifiedVersion(ctx context.Context, id coreapplication.ID) (int, error) {
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
func (s *Service) GetCharmByApplicationID(ctx context.Context, id coreapplication.ID) (
	internalcharm.Charm,
	domaincharm.CharmLocator,
	error,
) {
	if err := id.Validate(); err != nil {
		return nil, domaincharm.CharmLocator{}, internalerrors.Errorf("application ID: %w%w", err, errors.Hide(applicationerrors.ApplicationIDNotValid))
	}

	charm, err := s.st.GetCharmByApplicationID(ctx, id)
	if err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Trace(err)
	}

	// The charm needs to be decoded into the internalcharm.Charm type.

	metadata, err := decodeMetadata(charm.Metadata)
	if err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Trace(err)
	}

	manifest, err := decodeManifest(charm.Manifest)
	if err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Trace(err)
	}

	actions, err := decodeActions(charm.Actions)
	if err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Trace(err)
	}

	config, err := decodeConfig(charm.Config)
	if err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Trace(err)
	}

	lxdProfile, err := decodeLXDProfile(charm.LXDProfile)
	if err != nil {
		return nil, domaincharm.CharmLocator{}, errors.Trace(err)
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
	if providerID == "" {
		return errors.NotValidf("empty provider ID")
	}
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
			return false, errors.Trace(err)
		}
		appID, err := s.st.GetApplicationIDByName(ctx, appName)
		if err != nil {
			return false, errors.Trace(err)
		}
		var scaleInfo application.ScaleState
		err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
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
func (s *Service) GetApplicationLife(ctx context.Context, appName string) (corelife.Value, error) {
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
func (s *Service) SetApplicationScale(ctx context.Context, appName string, scale int) error {
	if scale < 0 {
		return fmt.Errorf("application scale %d not valid%w", scale, errors.Hide(applicationerrors.ScaleChangeInvalid))
	}
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return errors.Trace(err)
	}
	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
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
func (s *Service) GetApplicationScale(ctx context.Context, appName string) (int, error) {
	_, scale, err := s.getApplicationScaleAndID(ctx, appName)
	return scale, errors.Trace(err)
}

func (s *Service) getApplicationScaleAndID(ctx context.Context, appName string) (coreapplication.ID, int, error) {
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return "", -1, errors.Trace(err)
	}
	var scaleState application.ScaleState
	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error

		scaleState, err = s.st.GetApplicationScaleState(ctx, appID)
		return errors.Annotatef(err, "getting scaling state for %q", appName)
	})
	return appID, scaleState.Scale, errors.Trace(err)
}

// ChangeApplicationScale alters the existing scale by the provided change amount, returning the new amount.
// It returns an error satisfying [applicationerrors.ApplicationNotFoundError] if the application
// doesn't exist.
// This is used on CAAS models.
func (s *Service) ChangeApplicationScale(ctx context.Context, appName string, scaleChange int) (int, error) {
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return -1, errors.Trace(err)
	}
	var newScale int
	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
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
func (s *Service) SetApplicationScalingState(ctx context.Context, appName string, scaleTarget int, scaling bool) error {
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
func (s *Service) GetApplicationScalingState(ctx context.Context, appName string) (ScalingState, error) {
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return ScalingState{}, errors.Trace(err)
	}
	var scaleState application.ScaleState
	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
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
func (s *Service) GetApplicationsWithPendingCharmsFromUUIDs(ctx context.Context, uuids []coreapplication.ID) ([]coreapplication.ID, error) {
	if len(uuids) == 0 {
		return nil, nil
	}
	return s.st.GetApplicationsWithPendingCharmsFromUUIDs(ctx, uuids)
}

// GetAsyncCharmDownloadInfo returns a charm download info for the specified
// application. If the charm is already being downloaded, the method will
// return [applicationerrors.CharmAlreadyAvailable]. The charm download
// information is returned which includes the charm name, origin and the
// digest.
func (s *Service) GetAsyncCharmDownloadInfo(ctx context.Context, appID coreapplication.ID) (application.CharmDownloadInfo, error) {
	if err := appID.Validate(); err != nil {
		return application.CharmDownloadInfo{}, internalerrors.Errorf("application ID: %w", err)
	}

	return s.st.GetAsyncCharmDownloadInfo(ctx, appID)
}

// ResolveCharmDownload resolves the charm download slot for the specified
// application. The method will update the charm with the specified charm
// information.
// This returns [applicationerrors.CharmNotResolved] if the charm UUID isn't
// the same as the one that was reserved.
func (s *Service) ResolveCharmDownload(ctx context.Context, appID coreapplication.ID, resolve application.ResolveCharmDownload) error {
	if err := appID.Validate(); err != nil {
		return internalerrors.Errorf("application ID: %w", err)
	}

	// Although, we're resolving the charm download, we're calling the
	// reserve method to ensure that the charm download slot is still valid.
	// This has the added benefit of returning the charm hash, so that we can
	// verify the charm download. We don't want it to be passed in the resolve
	// charm download, in case the caller has the wrong hash.
	info, err := s.GetAsyncCharmDownloadInfo(ctx, appID)
	// There is nothing to do if the charm is already downloaded or resolved.
	if errors.Is(err, applicationerrors.CharmAlreadyAvailable) ||
		errors.Is(err, applicationerrors.CharmAlreadyResolved) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// If the charm UUID doesn't match, what was downloaded then we need to
	// return an error.
	if info.CharmUUID != resolve.CharmUUID {
		return applicationerrors.CharmNotResolved
	}

	// We need to ensure that charm sha256 hash matches the one that was
	// requested. If this is valid, we can then trust the sha384 hash, as we
	// have no provenance for it. In other words, we trust the sha384 hash, if
	// the sha256 hash is valid.
	if info.SHA256 != resolve.SHA256 {
		return applicationerrors.CharmHashMismatch
	}

	// Make sure it's actually a valid charm.
	charm, err := internalcharm.ReadCharmArchive(resolve.Path)
	if err != nil {
		return errors.Annotatef(err, "reading charm archive %q", resolve.Path)
	}

	// Encode the charm before we even attempt to store it. The charm storage
	// backend could be the other side of the globe.
	domainCharm, warnings, err := encodeCharm(charm)
	if err != nil {
		return errors.Annotatef(err, "encoding charm %q", resolve.Path)
	} else if len(warnings) > 0 {
		s.logger.Debugf("encoding charm %q: %v", resolve.Path, warnings)
	}

	// Use the hash from the reservation, incase the caller has the wrong hash.
	// The resulting objectStoreUUID will enable RI between the charm and the
	// object store.
	result, err := s.charmStore.Store(ctx, resolve.Path, resolve.Size, resolve.SHA384)
	if errors.Is(err, objectstoreerrors.ErrHashAndSizeAlreadyExists) {
		// If the hash already exists but has a different size, then we've
		// got a hash conflict. There isn't anything we can do about this, so
		// we'll return an error.
		return applicationerrors.CharmAlreadyExistsWithDifferentSize
	} else if err != nil {
		return errors.Trace(err)
	}

	// We must ensure that the objectstore UUID is valid.
	if err := result.ObjectStoreUUID.Validate(); err != nil {
		return internalerrors.Errorf("invalid object store UUID: %w", err)
	}

	// Resolve the charm download, which will set itself to available.
	return s.st.ResolveCharmDownload(ctx, info.CharmUUID, application.ResolvedCharmDownload{
		Actions:         domainCharm.Actions,
		LXDProfile:      domainCharm.LXDProfile,
		ObjectStoreUUID: result.ObjectStoreUUID,

		// This is correct, we want to use the unique name of the stored charm
		// as the archive path. Once every blob is storing the UUID, we can
		// remove the archive path, until, just use the unique name.
		ArchivePath: result.UniqueName,
	})
}

// ResolveControllerCharmDownload resolves the controller charm download slot.
func (s *Service) ResolveControllerCharmDownload(ctx context.Context, resolve application.ResolveControllerCharmDownload) (application.ResolvedControllerCharmDownload, error) {
	// Make sure it's actually a valid charm.
	charm, err := internalcharm.ReadCharmArchive(resolve.Path)
	if err != nil {
		return application.ResolvedControllerCharmDownload{}, errors.Annotatef(err, "reading charm archive %q", resolve.Path)
	}

	// Use the hash from the reservation, incase the caller has the wrong hash.
	// The resulting objectStoreUUID will enable RI between the charm and the
	// object store.
	result, err := s.charmStore.Store(ctx, resolve.Path, resolve.Size, resolve.SHA384)
	if errors.Is(err, objectstoreerrors.ErrHashAndSizeAlreadyExists) {
		// If the hash already exists but has a different size, then we've
		// got a hash conflict. There isn't anything we can do about this, so
		// we'll return an error.
		return application.ResolvedControllerCharmDownload{}, applicationerrors.CharmAlreadyExistsWithDifferentSize
	} else if err != nil {
		return application.ResolvedControllerCharmDownload{}, errors.Trace(err)
	}

	// We must ensure that the objectstore UUID is valid.
	if err := result.ObjectStoreUUID.Validate(); err != nil {
		return application.ResolvedControllerCharmDownload{}, internalerrors.Errorf("invalid object store UUID: %w", err)
	}

	// Resolve the charm download, which will set itself to available.
	return application.ResolvedControllerCharmDownload{
		Charm:           charm,
		ObjectStoreUUID: result.ObjectStoreUUID,

		// This is correct, we want to use the unique name of the stored charm
		// as the archive path. Once every blob is storing the UUID, we can
		// remove the archive path, until, just use the unique name.
		ArchivePath: result.UniqueName,
	}, nil
}

// GetApplicationsForRevisionUpdater returns all the applications for the
// revision updater. This will only return charmhub charms, for applications
// that are alive.
// This will return an empty slice if there are no applications.
func (s *Service) GetApplicationsForRevisionUpdater(ctx context.Context) ([]application.RevisionUpdaterApplication, error) {
	return s.st.GetApplicationsForRevisionUpdater(ctx)
}

// GetApplicationConfig returns the application config attributes for the
// configuration.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) GetApplicationConfig(ctx context.Context, appID coreapplication.ID) (config.ConfigAttributes, error) {
	if err := appID.Validate(); err != nil {
		return nil, internalerrors.Errorf("application ID: %w", err)
	}

	cfg, settings, err := s.st.GetApplicationConfigAndSettings(ctx, appID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make(config.ConfigAttributes)
	for k, v := range cfg {
		result[k] = v.Value
	}

	// Always return the trust setting, as it's a special case.
	result[coreapplication.TrustConfigOptionName] = settings.Trust

	return result, nil
}

// GetApplicationTrustSetting returns the application trust setting.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) GetApplicationTrustSetting(ctx context.Context, appID coreapplication.ID) (bool, error) {
	if err := appID.Validate(); err != nil {
		return false, internalerrors.Errorf("application ID: %w", err)
	}

	return s.st.GetApplicationTrustSetting(ctx, appID)
}

// UnsetApplicationConfigKeys removes the specified keys from the application
// config. If the key does not exist, it is ignored.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) UnsetApplicationConfigKeys(ctx context.Context, appID coreapplication.ID, keys []string) error {
	if err := appID.Validate(); err != nil {
		return internalerrors.Errorf("application ID: %w", err)
	}
	if len(keys) == 0 {
		return nil
	}
	return s.st.UnsetApplicationConfigKeys(ctx, appID, keys)
}

// SetApplicationConfig updates the application config with the specified
// values. If the key does not exist, it is created. If the key already exists,
// it is updated, if there is no value it is removed. With the caveat that
// application trust will be set to false.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
// If the charm does not exist, an error satisfying
// [applicationerrors.CharmNotFound] is returned, if this is the case, then
// the application is in a bad state and should be removed.
// If the charm config is not valid, an error satisfying
// [applicationerrors.CharmConfigNotValid] is returned.
func (s *Service) SetApplicationConfig(ctx context.Context, appID coreapplication.ID, newConfig map[string]string) error {
	if err := appID.Validate(); err != nil {
		return internalerrors.Errorf("application ID: %w", err)
	}

	// Get the charm config. This should be safe to do outside of a singular
	// transaction, as the charm config is immutable. So it will either be there
	// or not, and if it's not there we can return an error stating that.
	// Otherwise if it is there, but then is removed before we set the config, a
	// foreign key constraint will be violated, and we can return that as an
	// error.

	// Return back the charm UUID, so that we can verify that the charm
	// hasn't changed between this call and the transaction to set it.

	charmID, cfg, err := s.st.GetCharmConfigByApplicationID(ctx, appID)
	if err != nil {
		return internalerrors.Capture(err)
	}

	charmConfig, err := decodeConfig(cfg)
	if err != nil {
		return internalerrors.Capture(err)
	}

	// Grab the application settings, which is currently just the trust setting.
	trust, err := getTrustSettingFromConfig(newConfig)
	if err != nil {
		return internalerrors.Capture(err)
	}

	// Everything else from the newConfig is just application config. Treat it
	// as such.
	coercedConfig, err := charmConfig.ParseSettingsStrings(newConfig)
	if errors.Is(err, internalcharm.ErrUnknownOption) {
		return internalerrors.Errorf("%w: %w", applicationerrors.InvalidCharmConfig, err)
	} else if err != nil {
		return internalerrors.Capture(err)
	}

	// The encoded config is the application config, with the type of the
	// option. Encoding the type ensures that if the type changes during an
	// upgrade, we can prevent a runtime error during that phase.
	encodedConfig := make(map[string]application.ApplicationConfig, len(coercedConfig))
	for k, v := range coercedConfig {
		option, ok := charmConfig.Options[k]
		if !ok {
			// This should never happen, as we've verified the config is valid.
			// But if it does, then we should return an error.
			return internalerrors.Errorf("missing charm config, expected %q", k)
		}

		optionType, err := encodeOptionType(option.Type)
		if err != nil {
			return internalerrors.Capture(err)
		}

		encodedConfig[k] = application.ApplicationConfig{
			Value: v,
			Type:  optionType,
		}
	}

	return s.st.SetApplicationConfigAndSettings(ctx, appID, charmID, encodedConfig, application.ApplicationSettings{
		Trust: trust,
	})
}

func getTrustSettingFromConfig(cfg map[string]string) (bool, error) {
	trust, ok := cfg[coreapplication.TrustConfigOptionName]
	if ok {
		// Once we've got the trust value, we can remove it from the config.
		// Everything else is just application config.
		delete(cfg, coreapplication.TrustConfigOptionName)
	}

	// If the trust setting is not set, then we can just return false, as
	// parse bool will return an error for empty strings.
	if trust == "" {
		return false, nil
	}

	b, err := strconv.ParseBool(trust)
	if err != nil {
		return false, internalerrors.Errorf("parsing trust setting: %w", err)
	}
	return b, nil
}

func encodeApplicationConfig(cfg config.ConfigAttributes, charmConfig domaincharm.Config) (map[string]application.ApplicationConfig, error) {
	// If there is no config, then we can just return nil.
	if len(cfg) == 0 {
		return nil, nil
	}

	encodedConfig := make(map[string]application.ApplicationConfig, len(cfg))
	for k, v := range cfg {
		option, ok := charmConfig.Options[k]
		if !ok {
			// This should never happen, as we've verified the config is valid.
			// But if it does, then we should return an error.
			return nil, internalerrors.Errorf("missing charm config, expected %q", k)
		}

		encodedConfig[k] = application.ApplicationConfig{
			Value: v,
			Type:  option.Type,
		}
	}
	return encodedConfig, nil
}
