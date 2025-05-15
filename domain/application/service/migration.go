// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/clock"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/ipaddress"
	domainnetwork "github.com/juju/juju/domain/network"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// MigrationState is the state required for migrating applications.
type MigrationState interface {
	// GetApplicationsForExport returns all the applications in the model.
	GetApplicationsForExport(ctx context.Context) ([]application.ExportApplication, error)

	// GetApplicationUnitsForExport returns all the units for a given
	// application in the model.
	// If the application does not exist, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationUnitsForExport(ctx context.Context, appID coreapplication.ID) ([]application.ExportUnit, error)

	// GetSpaceUUIDByName returns the UUID of the space with the given name.
	// It returns an error satisfying [networkerrors.SpaceNotFound] if the provided
	//
	// space name doesn't exist.
	GetSpaceUUIDByName(ctx context.Context, name string) (network.Id, error)

	// InsertMigratingApplication inserts a migrating application. Returns as
	// error satisfying [applicationerrors.ApplicationAlreadyExists] if the
	// application already exists. If returns as error satisfying
	// [applicationerrors.CharmNotFound] if the charm for the application is
	// not found.
	InsertMigratingApplication(context.Context, string, application.InsertApplicationArgs) (coreapplication.ID, error)
}

// MigrationService provides the API for migrating applications.
type MigrationService struct {
	st                    State
	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	clock                 clock.Clock
	logger                logger.Logger
}

// NewMigrationService returns a new service reference wrapping the input state.
func NewMigrationService(
	st State,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	clock clock.Clock,
	logger logger.Logger,
) *MigrationService {
	return &MigrationService{
		st:                    st,
		storageRegistryGetter: storageRegistryGetter,
		clock:                 clock,
		logger:                logger,
	}
}

// GetCharmID returns a charm ID by name. It returns an error if the charm
// can not be found by the name.
// This can also be used as a cheap way to see if a charm exists without
// needing to load the charm metadata.
// Returns [applicationerrors.CharmNameNotValid] if the name is not valid, and
// [applicationerrors.CharmNotFound] if the charm is not found.
func (s *MigrationService) GetCharmID(ctx context.Context, args charm.GetCharmArgs) (corecharm.ID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !isValidCharmName(args.Name) {
		return "", applicationerrors.CharmNameNotValid
	}

	// Validate the source, it can only be charmhub or local.
	if args.Source != charm.CharmHubSource && args.Source != charm.LocalSource {
		return "", applicationerrors.CharmSourceNotValid
	}

	if rev := args.Revision; rev != nil && *rev >= 0 {
		return s.st.GetCharmID(ctx, args.Name, *rev, args.Source)
	}

	return "", applicationerrors.CharmNotFound
}

// GetCharmByApplicationName returns the charm using the application name.
// Calling this method will return all the data associated with the charm.
// It is not expected to call this method for all calls, instead use the move
// focused and specific methods. That's because this method is very expensive
// to call. This is implemented for the cases where all the charm data is
// needed; model migration, charm export, etc.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *MigrationService) GetCharmByApplicationName(ctx context.Context, name string) (internalcharm.Charm, charm.CharmLocator, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !isValidApplicationName(name) {
		return nil, charm.CharmLocator{}, applicationerrors.ApplicationNameNotValid
	}

	id, err := s.st.GetCharmIDByApplicationName(ctx, name)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	ch, _, err := s.st.GetCharm(ctx, id)
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

// GetApplications returns all the applications in the model.
func (s *MigrationService) GetApplications(ctx context.Context) ([]application.ExportApplication, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetApplicationsForExport(ctx)
}

// GetApplicationUnits returns all the units for the specified application.
// If the application does not exist, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *MigrationService) GetApplicationUnits(ctx context.Context, name string) ([]application.ExportUnit, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !isValidApplicationName(name) {
		return nil, applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationIDByName(ctx, name)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return s.st.GetApplicationUnitsForExport(ctx, appID)
}

// GetApplicationCharmOrigin returns the charm origin for the specified
// application name. If the application does not exist, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *MigrationService) GetApplicationCharmOrigin(ctx context.Context, name string) (application.CharmOrigin, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !isValidApplicationName(name) {
		return application.CharmOrigin{}, applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationIDByName(ctx, name)
	if err != nil {
		return application.CharmOrigin{}, errors.Capture(err)
	}

	return s.st.GetApplicationCharmOrigin(ctx, appID)
}

// GetApplicationConfigAndSettings returns the application config and settings
// for the specified application. This will return the application config and
// the settings in one config.ConfigAttributes object.
//
// If the application does not exist, a [applicationerrors.ApplicationNotFound]
// error is returned. If no config is set for the application, an empty config
// is returned.
func (s *MigrationService) GetApplicationConfigAndSettings(ctx context.Context, name string) (config.ConfigAttributes, application.ApplicationSettings, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !isValidApplicationName(name) {
		return nil, application.ApplicationSettings{}, applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationIDByName(ctx, name)
	if err != nil {
		return nil, application.ApplicationSettings{}, errors.Capture(err)
	}

	cfg, settings, err := s.st.GetApplicationConfigAndSettings(ctx, appID)
	if err != nil {
		return nil, application.ApplicationSettings{}, errors.Capture(err)
	}

	result := make(config.ConfigAttributes)
	for k, v := range cfg {
		result[k] = v.Value
	}
	return result, settings, nil
}

// GetApplicationConstraints returns the application constraints for the
// specified application name.
// Empty constraints are returned if no constraints exist for the given
// application ID.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *MigrationService) GetApplicationConstraints(ctx context.Context, name string) (coreconstraints.Value, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !isValidApplicationName(name) {
		return coreconstraints.Value{}, applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationIDByName(ctx, name)
	if err != nil {
		return coreconstraints.Value{}, errors.Capture(err)
	}

	cons, err := s.st.GetApplicationConstraints(ctx, appID)
	return constraints.EncodeConstraints(cons), errors.Capture(err)
}

// GetUnitUUIDByName returns the unit UUID for the specified unit name.
// If the unit does not exist, an error satisfying
// [applicationerrors.UnitNotFound] is returned.
func (s *MigrationService) GetUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := name.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	return s.st.GetUnitUUIDByName(ctx, name)
}

// GetApplicationScaleState returns the scale state of the specified
// application, returning an error satisfying
// [applicationerrors.ApplicationNotFound] if the application is not found.
func (s *MigrationService) GetApplicationScaleState(ctx context.Context, name string) (application.ScaleState, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !isValidApplicationName(name) {
		return application.ScaleState{}, applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationIDByName(ctx, name)
	if err != nil {
		return application.ScaleState{}, errors.Capture(err)
	}

	return s.st.GetApplicationScaleState(ctx, appID)
}

// ImportCAASApplication imports the specified CAAS application and units
// if required, returning an error satisfying
// [applicationerrors.ApplicationAlreadyExists] if the application already
// exists.
func (s *MigrationService) ImportCAASApplication(ctx context.Context, name string, args ImportApplicationArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, charmUUID, err := s.importApplication(ctx, name, args)
	if err != nil {
		return errors.Errorf("importing application %q: %w", name, err)
	}

	// TODO hml 1-May-25
	// Improve the efficiency of importing caas applications by touching
	// the application_scale table once, instead of three times. Once in
	// st.ImportApplication and the following two methods.
	if err := s.st.SetApplicationScalingState(ctx, name, args.ScaleState.ScaleTarget, args.ScaleState.Scaling); err != nil {
		return errors.Errorf("setting scale state for application %q: %w", name, err)
	}
	if err := s.st.SetDesiredApplicationScale(ctx, appID, args.ScaleState.Scale); err != nil {
		return errors.Errorf("setting desired scale for application %q: %w", name, err)
	}

	unitArgs, err := makeUnitArgs(args.Units, charmUUID)
	if err != nil {
		return errors.Errorf("creating unit args: %w", err)
	}

	return s.st.InsertMigratingCAASUnits(ctx, appID, unitArgs...)
}

// ImportIAASApplication imports the specified IAAS application and units
// if required, returning an error satisfying
// [applicationerrors.ApplicationAlreadyExists] if the application already
// exists.
func (s *MigrationService) ImportIAASApplication(ctx context.Context, name string, args ImportApplicationArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, charmUUID, err := s.importApplication(ctx, name, args)
	if err != nil {
		return errors.Errorf("importing application %q: %w", name, err)
	}

	unitArgs, err := makeUnitArgs(args.Units, charmUUID)
	if err != nil {
		return errors.Errorf("creating unit args: %w", err)
	}

	return s.st.InsertMigratingIAASUnits(ctx, appID, unitArgs...)
}

func (s *MigrationService) importApplication(
	ctx context.Context,
	name string,
	args ImportApplicationArgs,
) (coreapplication.ID, corecharm.ID, error) {
	if err := validateCharmAndApplicationParams(name, args.ReferenceName, args.Charm, args.CharmOrigin); err != nil {
		return "", "", errors.Errorf("invalid application args: %w", err)
	}

	appArg, err := makeInsertApplicationArg(args)
	if err != nil {
		return "", "", errors.Errorf("creating application args: %w", err)
	}

	appArg.Scale = len(args.Units)

	appID, err := s.st.InsertMigratingApplication(ctx, name, appArg)
	if err != nil {
		return "", "", errors.Errorf("creating application %q: %w", name, err)
	}

	charmUUID, err := s.st.GetCharmIDByApplicationName(ctx, name)
	if err != nil {
		return "", "", errors.Errorf("getting charm ID for application %q: %w", name, err)
	}

	if err := s.st.MergeExposeSettings(ctx, appID, args.ExposedEndpoints); err != nil {
		return "", "", errors.Errorf("setting expose settings for application %q: %w", name, err)
	}
	if err := s.st.SetApplicationConstraints(ctx, appID, constraints.DecodeConstraints(args.ApplicationConstraints)); err != nil {
		return "", "", errors.Errorf("setting application constraints for application %q: %w", name, err)
	}

	return appID, charmUUID, nil
}

func makeInsertApplicationArg(
	args ImportApplicationArgs,
) (application.InsertApplicationArgs, error) {
	// When encoding the charm, this will also validate the charm metadata,
	// when parsing it.
	ch, _, err := encodeCharm(args.Charm)
	if err != nil {
		return application.InsertApplicationArgs{}, errors.Errorf("encoding charm: %w", err)
	}

	revision := -1
	origin := args.CharmOrigin
	if origin.Revision != nil {
		revision = *origin.Revision
	}

	source, err := encodeCharmSource(origin.Source)
	if err != nil {
		return application.InsertApplicationArgs{}, errors.Errorf("encoding charm source: %w", err)
	}

	ch.Source = source
	ch.ReferenceName = args.ReferenceName
	ch.Revision = revision
	ch.Hash = origin.Hash
	ch.Architecture = encodeArchitecture(origin.Platform.Architecture)

	channelArg, platformArg, err := encodeChannelAndPlatform(origin)
	if err != nil {
		return application.InsertApplicationArgs{}, errors.Errorf("encoding charm origin: %w", err)
	}

	applicationConfig, err := encodeApplicationConfig(args.ApplicationConfig, ch.Config)
	if err != nil {
		return application.InsertApplicationArgs{}, errors.Errorf("encoding application config: %w", err)
	}

	return application.InsertApplicationArgs{
		Charm:            ch,
		Platform:         platformArg,
		Channel:          channelArg,
		EndpointBindings: args.EndpointBindings,
		Resources:        makeResourcesArgs(args.ResolvedResources),
		Config:           applicationConfig,
		Settings:         args.ApplicationSettings,
		PeerRelations:    args.PeerRelations,
	}, nil
}

// IsApplicationExposed returns whether the provided application is exposed or not.
//
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *MigrationService) IsApplicationExposed(ctx context.Context, appName string) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return false, errors.Capture(err)
	}

	return s.st.IsApplicationExposed(ctx, appID)
}

// GetExposedEndpoints returns map where keys are endpoint names (or the ""
// value which represents all endpoints) and values are ExposedEndpoint
// instances that specify which sources (spaces or CIDRs) can access the
// opened ports for each endpoint once the application is exposed.
//
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *MigrationService) GetExposedEndpoints(ctx context.Context, appName string) (map[string]application.ExposedEndpoint, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return s.st.GetExposedEndpoints(ctx, appID)
}

// GetSpaceUUIDByName returns the UUID of the space with the given name.
//
// It returns an error satisfying [networkerrors.SpaceNotFound] if the provided
// space name doesn't exist.
func (s *MigrationService) GetSpaceUUIDByName(ctx context.Context, name string) (network.Id, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetSpaceUUIDByName(ctx, name)
}

func makeUnitArgs(units []ImportUnitArg, charmUUID corecharm.ID) ([]application.ImportUnitArg, error) {
	unitArgs := make([]application.ImportUnitArg, len(units))
	for i, u := range units {

		arg := application.ImportUnitArg{
			UnitName:  u.UnitName,
			Machine:   u.Machine,
			Principal: u.Principal,
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
	return unitArgs, nil
}

func makeCloudContainerArg(unitName coreunit.Name, cloudContainer application.CloudContainerParams) *application.CloudContainer {
	result := &application.CloudContainer{
		ProviderID: cloudContainer.ProviderID,
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
				DeviceTypeID:      domainnetwork.DeviceTypeUnknown,
				VirtualPortTypeID: domainnetwork.NonVirtualPortType,
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

// RemoveImportedApplication removes an application that was imported. The
// application might be in an incomplete state, so it's important to remove
// as much of the application as possible, even on failure.
func (s *MigrationService) RemoveImportedApplication(ctx context.Context, name string) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// TODO (stickupkid): This is a placeholder for now, we need to implement
	// this method.
	return nil
}
