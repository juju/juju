// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strconv"

	"github.com/juju/collections/set"
	"github.com/juju/os/v2"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	coreerrors "github.com/juju/juju/core/errors"
	corelife "github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/secrets"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	internalcharm "github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// ApplicationState describes retrieval and persistence methods for
// applications.
type ApplicationState interface {
	// GetApplicationIDByName returns the application ID for the named application.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// CreateIAASApplication creates an application. Returns the application ID,
	// along with any machine names created.
	CreateIAASApplication(
		context.Context,
		string,
		application.AddIAASApplicationArg,
		[]application.AddUnitArg,
	) (coreapplication.ID, []coremachine.Name, error)

	// CreateCAASApplication creates an application, returning an error
	// satisfying [applicationerrors.ApplicationAlreadyExists] if the
	// application already exists. If returns as error satisfying
	// [applicationerrors.CharmNotFound] if the charm for the application is not
	// found.
	CreateCAASApplication(context.Context, string, application.AddCAASApplicationArg, []application.AddUnitArg) (coreapplication.ID, error)

	// GetModelType returns the model type for the underlying model. If the
	// model does not exist then an error satisfying [modelerrors.NotFound] will
	// be returned.
	// Deprecated: This method will be removed, as there should be no need to
	// determine the model type from the state or service. That's an artifact of
	// the caller to call the correct methods.
	GetModelType(context.Context) (coremodel.ModelType, error)

	// StorageDefaults returns the default storage sources for a model.
	StorageDefaults(context.Context) (domainstorage.StorageDefaults, error)

	// GetStoragePoolByName returns the storage pool with the specified name,
	// returning an error satisfying [storageerrors.PoolNotFoundError] if it
	// doesn't exist.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePoolDetails, error)

	// UpsertCloudService updates the cloud service for the specified application.
	// The following errors may be returned:
	// - [applicationerrors.ApplicationNotFound] if the application doesn't exist
	UpsertCloudService(ctx context.Context, appName, providerID string, sAddrs network.ProviderAddresses) error

	// GetUnitAndK8sServiceAddresses returns the addresses of the specified unit.
	// The addresses are taken by unioning the net node UUIDs of the cloud service
	// (if any) and the net node UUIDs of the unit, where each net node has an
	// associated address.
	// This apprach allows us to get the addresses regardless of the substrate
	// (k8s or machines).
	//
	// The following errors may be returned:
	// - [uniterrors.UnitNotFound] if the unit does not exist
	GetUnitAndK8sServiceAddresses(ctx context.Context, uuid coreunit.UUID) (network.SpaceAddresses, error)

	// GetUnitAddresses returns the addresses of the specified unit.
	//
	// The following errors may be returned:
	// - [uniterrors.UnitNotFound] if the unit does not exist
	GetUnitAddresses(ctx context.Context, uuid coreunit.UUID) (network.SpaceAddresses, error)

	// IsSubordinateApplication returns true if the application is a subordinate
	// application.
	// The following errors may be returned:
	// - [appliationerrors.ApplicationNotFound] if the application does not exist
	IsSubordinateApplication(context.Context, coreapplication.ID) (bool, error)

	// GetApplicationScaleState looks up the scale state of the specified
	// application, returning an error satisfying
	// [applicationerrors.ApplicationNotFound] if the application is not found.
	GetApplicationScaleState(context.Context, coreapplication.ID) (application.ScaleState, error)

	// GetApplicationUnitLife returns the life values for the specified units of
	// the given application. The supplied ids may belong to a different
	// application; the application name is used to filter.
	GetApplicationUnitLife(ctx context.Context, appName string, unitUUIDs ...coreunit.UUID) (map[coreunit.UUID]life.Life, error)

	// GetApplicationLife looks up the life of the specified application,
	// returning an error satisfying
	// [applicationerrors.ApplicationNotFoundError] if the application is not
	// found.
	GetApplicationLife(ctx context.Context, appName string) (coreapplication.ID, life.Life, error)

	// SetApplicationLife sets the life of the specified application.
	SetApplicationLife(context.Context, coreapplication.ID, life.Life) error

	// SetApplicationScalingState sets the scaling details for the given caas
	// application Scale is optional and is only set if not nil.
	SetApplicationScalingState(ctx context.Context, appName string, targetScale int, scaling bool) error

	// SetDesiredApplicationScale updates the desired scale of the specified
	// application.
	SetDesiredApplicationScale(context.Context, coreapplication.ID, int) error

	// UpdateApplicationScale updates the desired scale of an application by a
	// delta.
	// If the resulting scale is less than zero, an error satisfying
	// [applicationerrors.ScaleChangeInvalid] is returned.
	UpdateApplicationScale(ctx context.Context, appUUID coreapplication.ID, delta int) (int, error)

	// DeleteApplication deletes the specified application, returning an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application doesn't exist. If the application still has units, as error
	// satisfying [applicationerrors.ApplicationHasUnits] is returned.
	DeleteApplication(context.Context, string) error

	// GetCharmByApplicationID returns the charm, charm origin and charm
	// platform for the specified application ID.
	//
	// If the application does not exist, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetCharmByApplicationID(context.Context, coreapplication.ID) (charm.Charm, error)

	// GetCharmIDByApplicationName returns a charm ID by application name. It
	// returns an error if the charm can not be found by the name. This can also
	// be used as a cheap way to see if a charm exists without needing to load
	// the charm metadata.
	GetCharmIDByApplicationName(context.Context, string) (corecharm.ID, error)

	// SetApplicationCharm sets a new charm for the specified application using
	// the provided parameters and validates changes.
	SetApplicationCharm(ctx context.Context, id coreapplication.ID, params application.UpdateCharmParams) error

	// GetApplicationIDByUnitName returns the application ID for the named unit,
	// returning an error satisfying [applicationerrors.UnitNotFound] if the
	// unit doesn't exist.
	GetApplicationIDByUnitName(ctx context.Context, name coreunit.Name) (coreapplication.ID, error)

	// GetApplicationIDAndNameByUnitName returns the application ID and name for
	// the named unit, returning an error satisfying
	// [applicationerrors.UnitNotFound] if the unit doesn't exist.
	GetApplicationIDAndNameByUnitName(ctx context.Context, name coreunit.Name) (coreapplication.ID, string, error)

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

	// GetApplicationConfigWithDefaults returns the application config attributes
	// for the configuration, or their charm default if the config attribute is not
	// set.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationConfigWithDefaults(ctx context.Context, appID coreapplication.ID) (
		map[string]application.ApplicationConfig,
		error,
	)

	// GetApplicationTrustSetting returns the application trust setting.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationTrustSetting(ctx context.Context, appID coreapplication.ID) (bool, error)

	// UpdateApplicationConfigAndSettings sets the application config attributes
	// using the configuration, and sets the trust setting as part of the
	// application.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	UpdateApplicationConfigAndSettings(
		ctx context.Context,
		appID coreapplication.ID,
		config map[string]application.ApplicationConfig,
		settings application.UpdateApplicationSettingsArg,
	) error

	// UnsetApplicationConfigKeys removes the specified keys from the application
	// config. If the key does not exist, it is ignored.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	UnsetApplicationConfigKeys(ctx context.Context, appID coreapplication.ID, keys []string) error

	// GetApplicationConfigHash returns the SHA256 hash of the application config
	// for the specified application ID.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationConfigHash(ctx context.Context, appID coreapplication.ID) (string, error)

	// InitialWatchStatementUnitLife returns the initial namespace query for the
	// application unit life watcher.
	InitialWatchStatementUnitLife(appName string) (string, eventsource.NamespaceQuery)

	// InitialWatchStatementApplicationsWithPendingCharms returns the initial
	// namespace query for the applications with pending charms watcher.
	InitialWatchStatementApplicationsWithPendingCharms() (string, eventsource.NamespaceQuery)

	// InitialWatchStatementApplicationConfigHash returns the initial namespace
	// query for the application config hash watcher.
	InitialWatchStatementApplicationConfigHash(appName string) (string, eventsource.NamespaceQuery)

	// InitialWatchStatementUnitAddressesHash returns the initial namespace query
	// for the unit addresses hash watcher as well as the tables to be watched
	// (ip_address and application_endpoint)
	InitialWatchStatementUnitAddressesHash(appUUID coreapplication.ID, netNodeUUID string) (string, string, eventsource.NamespaceQuery)

	// InitialWatchStatementUnitInsertDeleteOnNetNode returns the initial namespace
	// query for unit insert and delete events on a specific net node, as well as
	// the watcher namespace to watch.
	InitialWatchStatementUnitInsertDeleteOnNetNode(netNodeUUID string) (string, eventsource.NamespaceQuery)

	// GetAddressesHash returns the sha256 hash of the application unit and cloud
	// service (if any) addresses along with the associated endpoint bindings.
	GetAddressesHash(ctx context.Context, appUUID coreapplication.ID, netNodeUUID string) (string, error)

	// GetNetNodeUUIDByUnitName returns the net node UUID for the named unit or the
	// cloud service associated with the unit's application. This method is meant
	// to be used in the WatchUnitAddressesHash watcher as a filter for ip address
	// changes.
	//
	// If the unit does not exist an error satisfying
	// [applicationerrors.UnitNotFound] will be returned.
	GetNetNodeUUIDByUnitName(ctx context.Context, name coreunit.Name) (string, error)

	// GetApplicationConstraints returns the application constraints for the
	// specified application ID.
	// Empty constraints are returned if no constraints exist for the given
	// application ID.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationConstraints(ctx context.Context, appID coreapplication.ID) (constraints.Constraints, error)

	// SetApplicationConstraints sets the application constraints for the
	// specified application ID.
	// This method overwrites the full constraints on every call.
	// If invalid constraints are provided (e.g. invalid container type or
	// non-existing space), a [applicationerrors.InvalidApplicationConstraints]
	// error is returned.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	SetApplicationConstraints(ctx context.Context, appID coreapplication.ID, cons constraints.Constraints) error

	// GetApplicationCharmOrigin returns the platform and channel for the
	// specified application ID.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationCharmOrigin(ctx context.Context, appID coreapplication.ID) (application.CharmOrigin, error)

	// NamespaceForWatchApplication returns the namespace identifier
	// for application watchers.
	NamespaceForWatchApplication() string
	// NamespaceForWatchApplicationConfig returns a namespace string identifier
	// for application configuration changes.
	NamespaceForWatchApplicationConfig() string

	// NamespaceForWatchApplicationScale returns the namespace identifier
	// for application scale change watchers.
	NamespaceForWatchApplicationScale() string

	// IsApplicationExposed returns whether the provided application is exposed or
	// not.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	IsApplicationExposed(ctx context.Context, appID coreapplication.ID) (bool, error)

	// NamespaceForWatchApplicationExposed returns the namespace identifier
	// for application exposed endpoints changes. The first return value is the
	// namespace for the application exposed endpoints to spaces table, and the
	// second is the namespace for the application exposed endpoints to CIDRs
	// table.
	NamespaceForWatchApplicationExposed() (string, string)

	// NamespaceForWatchUnitForLegacyUniter returns the namespace identifiers
	// for unit changes needed for the uniter. The first return value is the
	// namespace for the unit's inherent properties, the second is the namespace
	// of unit principals (used to watch for changes in subordinates), and the
	// third is the namespace for the unit's resolved mode.
	NamespaceForWatchUnitForLegacyUniter() (string, string, string)

	// GetApplicationEndpointBindings returns the mapping for each endpoint name and
	// the space ID it is bound to (or empty if unspecified). When no bindings are
	// stored for the application, defaults are returned.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationEndpointBindings(context.Context, coreapplication.ID) (map[string]string, error)

	// GetApplicationEndpointNames returns the names of the endpoints for the given
	// application.
	// The following errors may be returned:
	//   - [applicationerrors.ApplicationNotFound] is returned if the application
	//     doesn't exist.
	GetApplicationEndpointNames(context.Context, coreapplication.ID) ([]string, error)

	// ValidateEndpointBindingsForApplication
	ValidateEndpointBindingsForApplication(context.Context, coreapplication.ID, map[string]network.SpaceName) error

	// MergeApplicationEndpointBindings merge the provided bindings into the bindings
	// for the specified application.
	// The following errors may be returned:
	// - [applicationerrors.ApplicationNotFound] if the application does not exist
	MergeApplicationEndpointBindings(context.Context, coreapplication.ID, map[string]network.SpaceName) error

	// NamespaceForWatchNetNodeAddress returns the namespace identifier for
	// net node address changes, which is the ip_address table.
	NamespaceForWatchNetNodeAddress() string

	// GetExposedEndpoints returns map where keys are endpoint names (or the ""
	// value which represents all endpoints) and values are ExposedEndpoint
	// instances that specify which sources (spaces or CIDRs) can access the
	// opened ports for each endpoint once the application is exposed.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetExposedEndpoints(ctx context.Context, appID coreapplication.ID) (map[string]application.ExposedEndpoint, error)

	// UnsetExposeSettings removes the expose settings for the provided list of
	// endpoint names. If the resulting exposed endpoints map for the application
	// becomes empty after the settings are removed, the application will be
	// automatically unexposed.
	// If the provided set of endpoints is empty, all exposed endpoints of the
	// application will be removed.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	UnsetExposeSettings(ctx context.Context, appID coreapplication.ID, exposedEndpoints set.Strings) error

	// MergeExposeSettings marks the application as exposed and merges the provided
	// ExposedEndpoint details into the current set of expose settings. The merge
	// operation will overwrite expose settings for each existing endpoint name.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	MergeExposeSettings(ctx context.Context, appID coreapplication.ID, exposedEndpoints map[string]application.ExposedEndpoint) error

	// EndpointsExist returns an error satisfying
	// [applicationerrors.EndpointNotFound] if any of the provided endpoints do not
	// exist.
	EndpointsExist(ctx context.Context, appID coreapplication.ID, endpoints set.Strings) error

	// SpacesExist returns an error satisfying [networkerrors.SpaceNotFound] if any
	// of the provided spaces do not exist.
	SpacesExist(ctx context.Context, spaceUUIDs set.Strings) error

	// GetDeviceConstraints returns the device constraints for an application.
	GetDeviceConstraints(ctx context.Context, appID coreapplication.ID) (map[string]devices.Constraints, error)

	// ShouldAllowCharmUpgradeOnError indicates if the units of an application
	// should upgrade to the latest version of the application charm even if
	// they are in error state.
	//
	// An error satisfying [applicationerrors.ApplicationNotFoundError]
	// is returned if the application doesn't exist.
	ShouldAllowCharmUpgradeOnError(ctx context.Context, appName string) (bool, error)
}

func validateCharmAndApplicationParams(
	name, referenceName string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
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

	// Validate the origin of the charm.
	if err := origin.Validate(); err != nil {
		return errors.Errorf("%w: %v", applicationerrors.CharmOriginNotValid, err)
	}

	return nil
}

func validateDownloadInfoParams(
	source corecharm.Source,
	downloadInfo *charm.DownloadInfo,
) error {
	// If the origin is from charmhub, then we require the download info
	// to deploy.
	if source != corecharm.CharmHub {
		return nil
	}
	if downloadInfo == nil {
		return applicationerrors.CharmDownloadInfoNotFound
	}
	if err := downloadInfo.Validate(); err != nil {
		return errors.Errorf("download info: %w", err)
	}
	return nil
}

func validateCreateApplicationResourceParams(
	charm internalcharm.Charm,
	resolvedResources ResolvedResources,
	pendingResources []resource.UUID,
) error {
	charmResources := charm.Meta().Resources

	switch {
	case len(charmResources) == 0 && (len(resolvedResources) != 0 || len(pendingResources) != 0):
		return errors.Errorf("charm has resources which have not provided: %w",
			applicationerrors.InvalidResourceArgs)
	case len(charmResources) == 0:
		return nil
	case len(pendingResources) != 0 && len(resolvedResources) != 0:
		return errors.Errorf("cannot have both pending and resolved resources: %w",
			applicationerrors.InvalidResourceArgs)
	case len(pendingResources) > 0:
		// resolvedResources and pendingResources are mutually exclusive.
		// Only one should be provided based on the code path to CreateApplication.
		// AddCharm requires pending resources, resolved by the client.
		// DeployFromRepository requires resources resolved on the controller.
		return validatePendingResource(len(charmResources), pendingResources)
	case len(resolvedResources) > 0:
		return validateResolvedResources(charmResources, resolvedResources)
	default:
		return errors.Errorf("charm has resources which have not provided: %w",
			applicationerrors.InvalidResourceArgs)
	}
}

func validatePendingResource(charmResourceCount int, pendingResources []resource.UUID) error {
	if len(pendingResources) != charmResourceCount {
		return errors.Errorf("pending and charm resource counts are different: %w",
			applicationerrors.InvalidResourceArgs)
	}
	return nil
}

func validateResolvedResources(charmResources map[string]charmresource.Meta, resolvedResources ResolvedResources) error {
	// Validate consistency of resources origin and revision
	if err := resolvedResources.Validate(); err != nil {
		return err
	}

	// Validates that all charm resources are resolved
	appResourceSet := set.NewStrings()
	charmResourceSet := set.NewStrings()
	for _, res := range charmResources {
		charmResourceSet.Add(res.Name)
	}
	for _, res := range charmResources {
		appResourceSet.Add(res.Name)
	}
	unexpectedResources := appResourceSet.Difference(charmResourceSet)
	missingResources := charmResourceSet.Difference(appResourceSet)
	if !unexpectedResources.IsEmpty() {
		// This needs to be an error because it will cause a foreign constraint
		// failure on insert, which is less easy to understand.
		return errors.Errorf("unexpected resources %v: %w", unexpectedResources.Values(),
			applicationerrors.InvalidResourceArgs)
	}
	if !missingResources.IsEmpty() {
		// Some resources are defined in the charm but not given when trying
		// to create the application.
		return errors.Errorf("charm resources not resolved %v: %w", missingResources.Values(),
			applicationerrors.InvalidResourceArgs)
	}

	return nil
}

func validateDeviceConstraints(cons map[string]devices.Constraints, charmMeta *internalcharm.Meta) error {
	// For each provided device constraint, we must check that the charm for the
	// application to be created has the same device (same name) defined.
	for name, deviceConstraint := range cons {
		charmDevice, ok := charmMeta.Devices[name]
		if !ok {
			return errors.Errorf("charm %q has no device called %q", charmMeta.Name, name)
		}
		// Ensure the provided count is valid.
		if charmDevice.CountMin > 0 && int64(deviceConstraint.Count) < charmDevice.CountMin {
			return errors.Errorf("minimum device count is %d, %d specified", charmDevice.CountMin, deviceConstraint.Count)
		}
	}

	// Ensure all charm devices have device constraint specified.
	for name, charmDevice := range charmMeta.Devices {
		if _, ok := cons[name]; !ok && charmDevice.CountMin > 0 {
			return errors.Errorf("no constraints specified for device %q", name)
		}
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	id, err := s.st.GetApplicationIDByUnitName(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting application id: %w", err)
	}
	return id, nil
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

// makeStorageArgs creates a slice of ApplicationStorageArg from a map of storage directives.
func makeStorageArgs(storage map[string]storage.Directive) []application.ApplicationStorageArg {
	var result []application.ApplicationStorageArg
	for name, stor := range storage {
		result = append(result, application.ApplicationStorageArg{
			Name:           corestorage.Name(name),
			PoolNameOrType: stor.Pool,
			Size:           stor.Size,
			Count:          stor.Count,
		})
	}
	return result
}

// DeleteApplication deletes the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// If the application still has units, as error satisfying [applicationerrors.ApplicationHasUnits]
// is returned.
func (s *Service) DeleteApplication(ctx context.Context, name string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.DeleteApplication(ctx, name); err != nil {
		return errors.Errorf("deleting application %q: %w", name, err)
	}
	return nil
}

// DestroyApplication prepares an application for removal from the model
// returning an error  satisfying [applicationerrors.ApplicationNotFoundError]
// if the application doesn't exist.
func (s *Service) DestroyApplication(ctx context.Context, appName string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil
	} else if err != nil {
		return errors.Errorf("getting application ID: %w", err)
	}
	// For now, all we do is advance the application's life to Dying.
	err = s.st.SetApplicationLife(ctx, appID, life.Dying)
	if err != nil {
		return errors.Errorf("destroying application %q: %w", appName, err)
	}
	return nil
}

// MarkApplicationDead is called by the cleanup worker if a mongo
// destroy operation sets the application to dead.
// TODO(units): remove when everything is in dqlite.
func (s *Service) MarkApplicationDead(ctx context.Context, appName string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return errors.Errorf("getting application ID: %w", err)
	}
	err = s.st.SetApplicationLife(ctx, appID, life.Dead)
	if err != nil {
		return errors.Errorf("setting application %q life to Dead: %w", appName, err)
	}
	return nil
}

// SetApplicationCharm sets a new charm for the application, validating that aspects such
// as storage are still viable with the new charm.
func (s *Service) SetApplicationCharm(ctx context.Context, appName string, params application.UpdateCharmParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return errors.Errorf("getting application ID: %w", err)
	}
	err = s.st.SetApplicationCharm(ctx, appID, params)
	if err != nil {
		return errors.Errorf("setting application %q charm: %w", appName, err)
	}
	return nil
}

// GetApplicationIDByName returns an application ID by application name. It
// returns an error if the application can not be found by the name.
//
// Returns [applicationerrors.ApplicationNameNotValid] if the name is not valid,
// and [applicationerrors.ApplicationNotFound] if the application is not found.
func (s *Service) GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !isValidApplicationName(name) {
		return "", applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationIDByName(ctx, name)
	if err != nil {
		return "", errors.Capture(err)
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
func (s *Service) GetCharmLocatorByApplicationName(ctx context.Context, name string) (charm.CharmLocator, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !isValidApplicationName(name) {
		return charm.CharmLocator{}, applicationerrors.ApplicationNameNotValid
	}

	charmID, err := s.st.GetCharmIDByApplicationName(ctx, name)
	if err != nil {
		return charm.CharmLocator{}, errors.Capture(err)
	}

	locator, err := s.getCharmLocatorByID(ctx, charmID)
	return locator, errors.Capture(err)
}

// GetCharmModifiedVersion looks up the charm modified version of the given
// application.
//
// Returns [applicationerrors.ApplicationNotFound] if the application is not found.
func (s *Service) GetCharmModifiedVersion(ctx context.Context, id coreapplication.ID) (int, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
// [applicationerrors.ApplicationNotFound] is returned. If the application name
// is not valid, an error satisfying [applicationerrors.ApplicationNameNotValid]
// is returned.
func (s *Service) GetCharmByApplicationID(ctx context.Context, id coreapplication.ID) (
	internalcharm.Charm,
	charm.CharmLocator,
	error,
) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := id.Validate(); err != nil {
		return nil, charm.CharmLocator{}, errors.Errorf("application ID: %w", err).Add(applicationerrors.ApplicationIDNotValid)
	}

	ch, err := s.st.GetCharmByApplicationID(ctx, id)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	// The charm needs to be decoded into the internalcharm.Charm type.

	metadata, err := decodeMetadata(ch.Metadata)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	manifest, err := decodeManifest(ch.Manifest)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	actions, err := decodeActions(ch.Actions)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	config, err := decodeConfig(ch.Config)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	lxdProfile, err := decodeLXDProfile(ch.LXDProfile)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	locator := charm.CharmLocator{
		Name:         ch.ReferenceName,
		Revision:     ch.Revision,
		Source:       ch.Source,
		Architecture: ch.Architecture,
	}

	return internalcharm.NewCharmBase(
		&metadata,
		&manifest,
		&config,
		&actions,
		&lxdProfile,
	), locator, nil
}

// UpsertCloudService updates the cloud service for the specified application.
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound] if the application doesn't exist
func (s *Service) UpdateCloudService(ctx context.Context, appName, providerID string, sAddrs network.ProviderAddresses) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if providerID == "" {
		return errors.Errorf("empty provider ID %w", coreerrors.NotValid)
	}
	return s.st.UpsertCloudService(ctx, appName, providerID, sAddrs)
}

// Broker provides access to the k8s cluster to guery the scale
// of a specified application.
type Broker interface {
	Application(string, caas.DeploymentType) caas.Application
}

// GetApplicationLife looks up the life of the specified application, returning
// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
// application is not found.
func (s *Service) GetApplicationLife(ctx context.Context, appName string) (corelife.Value, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	_, appLife, err := s.st.GetApplicationLife(ctx, appName)
	if err != nil {
		return "", errors.Errorf("getting life for %q: %w", appName, err)
	}
	return appLife.Value()
}

// IsSubordinateApplication returns true if the application is a subordinate
// application.
// The following errors may be returned:
// - [appliationerrors.ApplicationNotFound] if the application does not exist
func (s *Service) IsSubordinateApplication(ctx context.Context, appUUID coreapplication.ID) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	subordinate, err := s.st.IsSubordinateApplication(ctx, appUUID)
	if err != nil {
		return false, errors.Capture(err)
	}
	return subordinate, nil
}

// IsSubordinateApplicationByName returns true if the application is a subordinate
// application.
// The following errors may be returned:
// - [appliationerrors.ApplicationNotFound] if the application does not exist
func (s *Service) IsSubordinateApplicationByName(ctx context.Context, appName string) (bool, error) {
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return false, errors.Capture(err)
	}
	return s.IsSubordinateApplication(ctx, appID)
}

// SetApplicationScale sets the application's desired scale value, returning an error
// satisfying [applicationerrors.ApplicationNotFound] if the application is not found.
// This is used on CAAS models.
func (s *Service) SetApplicationScale(ctx context.Context, appName string, scale int) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if scale < 0 {
		return errors.Errorf("application scale %d not valid", scale).Add(applicationerrors.ScaleChangeInvalid)
	}
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return errors.Capture(err)
	}
	appScale, err := s.st.GetApplicationScaleState(ctx, appID)
	if err != nil {
		return errors.Errorf("getting application scale state for app %q: %w", appID, err)
	}
	s.logger.Tracef(ctx,
		"SetScale DesiredScale %v -> %v", appScale.Scale, scale,
	)
	err = s.st.SetDesiredApplicationScale(ctx, appID, scale)
	if err != nil {
		return errors.Errorf("setting scale for application %q: %w", appName, err)
	}
	return nil
}

// GetApplicationScale returns the desired scale of an application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// This is used on CAAS models.
func (s *Service) GetApplicationScale(ctx context.Context, appName string) (int, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return -1, errors.Capture(err)
	}
	scaleState, err := s.st.GetApplicationScaleState(ctx, appID)
	if err != nil {
		return -1, errors.Errorf("getting scaling state for %q: %w", appName, err)
	}
	return scaleState.Scale, nil
}

// ShouldAllowCharmUpgradeOnError indicates if the units of an application should
// upgrade to the latest version of the application charm even if they are in
// error state.
//
// An error satisfying [applicationerrors.ApplicationNotFoundError]
// is returned if the application doesn't exist.
func (s *Service) ShouldAllowCharmUpgradeOnError(ctx context.Context, appName string) (bool, error) {
	ok, err := s.st.ShouldAllowCharmUpgradeOnError(ctx, appName)
	if err != nil {
		return false, errors.Capture(err)
	}

	return ok, nil
}

// ChangeApplicationScale alters the existing scale by the provided change amount, returning the new amount.
// It returns an error satisfying [applicationerrors.ApplicationNotFoundError] if the application
// doesn't exist.
// This is used on CAAS models.
func (s *Service) ChangeApplicationScale(ctx context.Context, appName string, scaleChange int) (int, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return -1, errors.Capture(err)
	}

	newScale, err := s.st.UpdateApplicationScale(ctx, appID, scaleChange)
	if err != nil {
		return -1, errors.Errorf("changing scaling state for %q: %w", appName, err)
	}
	return newScale, nil
}

// SetApplicationScalingState updates the scale state of an application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// This is used on CAAS models.
func (s *Service) SetApplicationScalingState(ctx context.Context, appName string, scaleTarget int, scaling bool) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.SetApplicationScalingState(ctx, appName, scaleTarget, scaling); err != nil {
		return errors.Errorf("updating scaling state for %q: %w", appName, err)
	}
	return nil
}

// GetApplicationScalingState returns the scale state of an application,
// returning an error satisfying [applicationerrors.ApplicationNotFoundError] if
// the application doesn't exist. This is used on CAAS models.
func (s *Service) GetApplicationScalingState(ctx context.Context, appName string) (ScalingState, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return ScalingState{}, errors.Capture(err)
	}
	scaleState, err := s.st.GetApplicationScaleState(ctx, appID)
	if err != nil {
		return ScalingState{}, errors.Errorf("getting scaling state for %q: %w", appName, err)
	}
	return ScalingState{
		ScaleTarget: scaleState.ScaleTarget,
		Scaling:     scaleState.Scaling,
	}, nil
}

// GetApplicationsWithPendingCharmsFromUUIDs returns the application UUIDs that
// have pending charms from the provided UUIDs. If there are no applications
// with pending status charms, then those applications are ignored.
func (s *Service) GetApplicationsWithPendingCharmsFromUUIDs(ctx context.Context, uuids []coreapplication.ID) ([]coreapplication.ID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return application.CharmDownloadInfo{}, errors.Errorf("application ID: %w", err)
	}

	return s.st.GetAsyncCharmDownloadInfo(ctx, appID)
}

// ResolveCharmDownload resolves the charm download slot for the specified
// application. The method will update the charm with the specified charm
// information.
// This returns [applicationerrors.CharmNotResolved] if the charm UUID isn't
// the same as the one that was reserved.
func (s *Service) ResolveCharmDownload(ctx context.Context, appID coreapplication.ID, resolve application.ResolveCharmDownload) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return errors.Errorf("application ID: %w", err)
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
		return errors.Capture(err)
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
		return errors.Errorf("reading charm archive %q: %w", resolve.Path, err)
	}

	// Encode the charm before we even attempt to store it. The charm storage
	// backend could be the other side of the globe.
	domainCharm, warnings, err := encodeCharm(charm)
	if err != nil {
		return errors.Errorf("encoding charm %q: %w", resolve.Path, err)
	} else if len(warnings) > 0 {
		s.logger.Debugf(ctx, "encoding charm %q: %v", resolve.Path, warnings)
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
		return errors.Capture(err)
	}

	// We must ensure that the objectstore UUID is valid.
	if err := result.ObjectStoreUUID.Validate(); err != nil {
		return errors.Errorf("invalid object store UUID: %w", err)
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Make sure it's actually a valid charm.
	charm, err := internalcharm.ReadCharmArchive(resolve.Path)
	if err != nil {
		return application.ResolvedControllerCharmDownload{}, errors.Errorf("reading charm archive %q: %w", resolve.Path, err)
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
		return application.ResolvedControllerCharmDownload{}, errors.Capture(err)
	}

	// We must ensure that the objectstore UUID is valid.
	if err := result.ObjectStoreUUID.Validate(); err != nil {
		return application.ResolvedControllerCharmDownload{}, errors.Errorf("invalid object store UUID: %w", err)
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.st.GetApplicationsForRevisionUpdater(ctx)
}

// GetApplicationConfig returns the application config attributes for the
// configuration.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) GetApplicationConfig(ctx context.Context, appID coreapplication.ID) (config.ConfigAttributes, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return nil, errors.Errorf("application ID: %w", err)
	}

	cfg, settings, err := s.st.GetApplicationConfigAndSettings(ctx, appID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(config.ConfigAttributes)
	for k, v := range cfg {
		result[k] = v.Value
	}

	// Always return the trust setting, as it's a special case.
	result[coreapplication.TrustConfigOptionName] = settings.Trust

	return result, nil
}

// GetApplicationConfigWithDefaults returns the application config attributes
// for the configuration, or their charm default if the config attribute is not
// set.
//
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) GetApplicationConfigWithDefaults(ctx context.Context, appID coreapplication.ID) (config.ConfigAttributes, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return nil, errors.Errorf("application ID: %w", err)
	}

	cfg, err := s.st.GetApplicationConfigWithDefaults(ctx, appID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(config.ConfigAttributes)
	for k, v := range cfg {
		result[k] = v.Value
	}

	return result, nil
}

// GetApplicationTrustSetting returns the application trust setting.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) GetApplicationTrustSetting(ctx context.Context, appID coreapplication.ID) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return false, errors.Errorf("application ID: %w", err)
	}

	return s.st.GetApplicationTrustSetting(ctx, appID)
}

// GetApplicationCharmOrigin returns the charm origin for the specified
// application name. If the application does not exist, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) GetApplicationCharmOrigin(ctx context.Context, name string) (application.CharmOrigin, error) {
	if !isValidApplicationName(name) {
		return application.CharmOrigin{}, applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationIDByName(ctx, name)
	if err != nil {
		return application.CharmOrigin{}, errors.Capture(err)
	}

	return s.st.GetApplicationCharmOrigin(ctx, appID)
}

// GetApplicationAndCharmConfig returns the application and charm config for the
// specified application ID.
func (s *Service) GetApplicationAndCharmConfig(ctx context.Context, appID coreapplication.ID) (ApplicationConfig, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return ApplicationConfig{}, errors.Errorf("application ID: %w", err)
	}

	appConfig, settings, err := s.st.GetApplicationConfigAndSettings(ctx, appID)
	if err != nil {
		return ApplicationConfig{}, errors.Capture(err)
	}

	result := make(config.ConfigAttributes)
	for k, v := range appConfig {
		result[k] = v.Value
	}

	charmID, charmConfig, err := s.st.GetCharmConfigByApplicationID(ctx, appID)
	if err != nil {
		return ApplicationConfig{}, errors.Capture(err)
	}

	decodedCharmConfig, err := decodeConfig(charmConfig)
	if err != nil {
		return ApplicationConfig{}, errors.Errorf("decoding charm config: %w", err)
	}

	subordinate, err := s.st.IsSubordinateCharm(ctx, charmID)
	if err != nil {
		return ApplicationConfig{}, errors.Errorf("checking if charm is subordinate: %w", err)
	}

	origin, err := s.st.GetApplicationCharmOrigin(ctx, appID)
	if err != nil {
		return ApplicationConfig{}, errors.Errorf("getting charm origin: %w", err)
	}

	decodedCharmOrigin, err := decodeCharmOrigin(origin)
	if err != nil {
		return ApplicationConfig{}, errors.Errorf("decoding charm origin: %w", err)
	}

	return ApplicationConfig{
		CharmName:         origin.Name,
		CharmOrigin:       decodedCharmOrigin,
		CharmConfig:       decodedCharmConfig,
		ApplicationConfig: result,
		Trust:             settings.Trust,
		Principal:         !subordinate,
	}, nil
}

// UnsetApplicationConfigKeys removes the specified keys from the application
// config. If the key does not exist, it is ignored.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) UnsetApplicationConfigKeys(ctx context.Context, appID coreapplication.ID, keys []string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return errors.Errorf("application ID: %w", err)
	}
	if len(keys) == 0 {
		return nil
	}
	return s.st.UnsetApplicationConfigKeys(ctx, appID, keys)
}

// UpdateApplicationConfig updates the application config with the specified
// values. If the key does not exist, it is created. If the key already exists,
// it is updated, if there is no value it is removed. With the caveat that
// application trust will be set to false.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
// If the application config is not valid, an error satisfying
// [applicationerrors.InvalidApplicationConfig] is returned.
func (s *Service) UpdateApplicationConfig(ctx context.Context, appID coreapplication.ID, newConfig map[string]string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return errors.Errorf("application ID: %w", err)
	}

	// Get the charm config. This should be safe to do outside of a singular
	// transaction, as the charm config is immutable. So it will either be there
	// or not, and if it's not there we can return an error stating that.
	// Otherwise if it is there, but then is removed before we set the config, a
	// foreign key constraint will be violated, and we can return that as an
	// error.

	// Return back the charm UUID, so that we can verify that the charm
	// hasn't changed between this call and the transaction to set it.

	_, cfg, err := s.st.GetCharmConfigByApplicationID(ctx, appID)
	if err != nil {
		return errors.Capture(err)
	}

	charmConfig, err := decodeConfig(cfg)
	if err != nil {
		return errors.Capture(err)
	}

	// Grab the application settings, which is currently just the trust setting.
	trust, err := getTrustSettingFromConfig(newConfig)
	if err != nil {
		return errors.Capture(err)
	}

	// Everything else from the newConfig is just application config. Treat it
	// as such.
	coercedConfig, err := charmConfig.ParseSettingsStrings(newConfig)
	if errors.Is(err, internalcharm.ErrUnknownOption) {
		return errors.Errorf("%w: %w", applicationerrors.InvalidApplicationConfig, err)
	} else if err != nil {
		return errors.Capture(err)
	}

	// Validate the secret config.
	if err := validateSecretConfig(charmConfig, coercedConfig); err != nil {
		return errors.Capture(err)
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
			return errors.Errorf("missing charm config, expected %q", k)
		}

		optionType, err := encodeOptionType(option.Type)
		if err != nil {
			return errors.Capture(err)
		}

		encodedConfig[k] = application.ApplicationConfig{
			Value: v,
			Type:  optionType,
		}
	}

	return s.st.UpdateApplicationConfigAndSettings(ctx, appID, encodedConfig, application.UpdateApplicationSettingsArg{
		Trust: trust,
	})
}

// GetApplicationConstraints returns the application constraints for the
// specified application ID.
// Empty constraints are returned if no constraints exist for the given
// application ID.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) GetApplicationConstraints(ctx context.Context, appID coreapplication.ID) (coreconstraints.Value, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return coreconstraints.Value{}, errors.Errorf("application ID: %w", err)
	}

	cons, err := s.st.GetApplicationConstraints(ctx, appID)
	return constraints.EncodeConstraints(cons), errors.Capture(err)
}

// GetApplicationEndpointBindings returns the mapping for each endpoint name and
// the space ID it is bound to (or empty if unspecified). When no bindings are
// stored for the application, defaults are returned.
//
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) GetApplicationEndpointBindings(ctx context.Context, appID coreapplication.ID) (map[string]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return nil, errors.Errorf("validating application ID: %w", err)
	}

	bindings, err := s.st.GetApplicationEndpointBindings(ctx, appID)
	return bindings, errors.Capture(err)
}

// GetApplicationEndpointNames returns the names of the endpoints for the given
// application.
// The following errors may be returned:
//   - [applicationerrors.ApplicationNotFound] is returned if the application
//     doesn't exist.
func (s *Service) GetApplicationEndpointNames(ctx context.Context, appUUID coreapplication.ID) ([]string, error) {
	if err := appUUID.Validate(); err != nil {
		return nil, errors.Errorf("validating application UUID: %w", err)
	}

	eps, err := s.st.GetApplicationEndpointNames(ctx, appUUID)
	return eps, errors.Capture(err)
}

// MergeApplicationEndpointBindings merge the provided bindings into the bindings
// for the specified application.
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound] if the application does not exist
func (s *Service) MergeApplicationEndpointBindings(ctx context.Context, appID coreapplication.ID, bindings map[string]network.SpaceName, force bool) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return errors.Errorf("validating application ID: %w", err)
	}

	if !force {
		if err := s.st.ValidateEndpointBindingsForApplication(ctx, appID, bindings); err != nil {
			return errors.Errorf("validating endpoint bindings: %w", err)
		}
	}

	return s.st.MergeApplicationEndpointBindings(ctx, appID, bindings)
}

// GetDeviceConstraints returns the device constraints for an application.
//
// If the application is dead, [applicationerrors.ApplicationIsDead] is returned.
// If the application is not found, [applicationerrors.ApplicationNotFound]
// is returned.
func (s *Service) GetDeviceConstraints(ctx context.Context, name string) (map[string]devices.Constraints, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, err := s.st.GetApplicationIDByName(ctx, name)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return s.st.GetDeviceConstraints(ctx, appID)
}

func getTrustSettingFromConfig(cfg map[string]string) (*bool, error) {
	trust, ok := cfg[coreapplication.TrustConfigOptionName]
	if !ok {
		// trust is not included, so we should not update it.
		return nil, nil
	}

	// Once we've got the trust value, we can remove it from the config.
	// Everything else is just application config.
	delete(cfg, coreapplication.TrustConfigOptionName)

	b, err := strconv.ParseBool(trust)
	if err != nil {
		return nil, errors.Errorf("parsing trust setting: %w", err)
	}
	return &b, nil
}

func encodeApplicationConfig(cfg config.ConfigAttributes, charmConfig charm.Config) (map[string]application.ApplicationConfig, error) {
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
			return nil, errors.Errorf("missing charm config, expected %q", k)
		}

		encodedConfig[k] = application.ApplicationConfig{
			Value: v,
			Type:  option.Type,
		}
	}
	return encodedConfig, nil
}

func validateSecretConfig(chCfg internalcharm.Config, cfg internalcharm.Settings) error {
	for name, value := range cfg {
		option, ok := chCfg.Options[name]
		if !ok {
			// This should never happen.
			return errors.Errorf("unsupported option %q %w", name, coreerrors.NotValid)
		}
		if option.Type == "secret" {
			uriStr, ok := value.(string)
			if !ok {
				return applicationerrors.InvalidSecretConfig
			}
			if uriStr == "" {
				return nil
			}
			_, err := secrets.ParseURI(uriStr)
			if err != nil {
				return errors.Errorf("invalid secret URI for option %q: %w", name, err)
			}
			return nil
		}
	}
	return nil
}

func decodeCharmOrigin(origin application.CharmOrigin) (corecharm.Origin, error) {
	decodedSource, err := decodeCharmSource(origin.Source)
	if err != nil {
		return corecharm.Origin{}, errors.Errorf("decoding charm source: %w", err)
	}

	decodePlatform, err := decodePlatform(origin.Platform)
	if err != nil {
		return corecharm.Origin{}, errors.Errorf("decoding platform: %w", err)
	}

	decodedChannel, err := decodeChannel(origin.Channel)
	if err != nil {
		return corecharm.Origin{}, errors.Errorf("decoding channel: %w", err)
	}

	return corecharm.Origin{
		Source:   decodedSource,
		Channel:  decodedChannel,
		Platform: decodePlatform,
	}, nil
}

func decodeCharmSource(source charm.CharmSource) (corecharm.Source, error) {
	switch source {
	case charm.CharmHubSource:
		return corecharm.CharmHub, nil
	case charm.LocalSource:
		return corecharm.Local, nil
	default:
		return "", errors.Errorf("unsupported charm source type %q", source)
	}
}

func decodePlatform(platform deployment.Platform) (corecharm.Platform, error) {
	decodedArch, err := decodeArchitecture(platform.Architecture)
	if err != nil {
		return corecharm.Platform{}, errors.Errorf("decoding architecture: %w", err)
	}

	decodedOS, err := decodeOS(platform.OSType)
	if err != nil {
		return corecharm.Platform{}, errors.Errorf("decoding OS: %w", err)
	}

	return corecharm.Platform{
		OS:           decodedOS.String(),
		Architecture: decodedArch,
		Channel:      platform.Channel,
	}, nil
}

func decodeArchitecture(a architecture.Architecture) (arch.Arch, error) {
	switch a {
	case architecture.AMD64:
		return arch.AMD64, nil
	case architecture.ARM64:
		return arch.ARM64, nil
	case architecture.PPC64EL:
		return arch.PPC64EL, nil
	case architecture.RISCV64:
		return arch.RISCV64, nil
	case architecture.S390X:
		return arch.S390X, nil
	default:
		return "", errors.Errorf("unsupported architecture %q", a)
	}
}

func decodeOS(osType deployment.OSType) (os.OSType, error) {
	switch osType {
	case deployment.Ubuntu:
		return os.Ubuntu, nil
	default:
		return -1, errors.Errorf("unsupported OS type %q", osType)
	}
}

func decodeChannel(channel *deployment.Channel) (*internalcharm.Channel, error) {
	if channel == nil {
		return nil, nil
	}

	risk, err := decodeRisk(channel.Risk)
	if err != nil {
		return nil, errors.Errorf("decoding risk: %w", err)
	}

	ch, err := internalcharm.MakeChannel(channel.Track, risk.String(), channel.Branch)
	if err != nil {
		return nil, errors.Errorf("making channel: %w", err)
	}
	return &ch, nil
}

func decodeRisk(r deployment.ChannelRisk) (internalcharm.Risk, error) {
	switch r {
	case deployment.RiskStable:
		return internalcharm.Stable, nil
	case deployment.RiskCandidate:
		return internalcharm.Candidate, nil
	case deployment.RiskBeta:
		return internalcharm.Beta, nil
	case deployment.RiskEdge:
		return internalcharm.Edge, nil
	default:
		return "", errors.Errorf("unsupported risk %q", r)
	}
}
