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
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/linklayerdevice"
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
	return s.st.GetApplicationsForExport(ctx)
}

// GetApplicationUnits returns all the units for the specified application.
// If the application does not exist, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *MigrationService) GetApplicationUnits(ctx context.Context, name string) ([]application.ExportUnit, error) {
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
	if err := name.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	return s.st.GetUnitUUIDByName(ctx, name)
}

// GetApplicationScaleState returns the scale state of the specified
// application, returning an error satisfying
// [applicationerrors.ApplicationNotFound] if the application is not found.
func (s *MigrationService) GetApplicationScaleState(ctx context.Context, name string) (application.ScaleState, error) {
	if !isValidApplicationName(name) {
		return application.ScaleState{}, applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationIDByName(ctx, name)
	if err != nil {
		return application.ScaleState{}, errors.Capture(err)
	}

	return s.st.GetApplicationScaleState(ctx, appID)
}

// ImportApplication imports the specified application and units if required,
// returning an error satisfying [applicationerrors.ApplicationAlreadyExists]
// if the application already exists.
func (s *MigrationService) ImportApplication(ctx context.Context, name string, args ImportApplicationArgs) error {
	if err := validateCharmAndApplicationParams(name, args.ReferenceName, args.Charm, args.CharmOrigin, args.DownloadInfo); err != nil {
		return errors.Errorf("invalid application args: %w", err)
	}

	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return errors.Errorf("getting model type: %w", err)
	}
	appArg, err := makeCreateApplicationArgs(ctx, s.st, s.storageRegistryGetter, modelType, args.Charm, args.CharmOrigin, AddApplicationArgs{
		ReferenceName:       args.ReferenceName,
		DownloadInfo:        args.DownloadInfo,
		ApplicationConfig:   args.ApplicationConfig,
		ApplicationSettings: args.ApplicationSettings,
	})
	if err != nil {
		return errors.Errorf("creating application args: %w", err)
	}

	appArg.Scale = len(args.Units)

	appID, err := s.st.CreateApplication(ctx, name, appArg, nil)
	if err != nil {
		return errors.Errorf("creating application %q: %w", name, err)
	}

	charmUUID, err := s.st.GetCharmIDByApplicationName(ctx, name)
	if err != nil {
		return errors.Errorf("getting charm ID for application %q: %w", name, err)
	}

	unitArgs, err := makeUnitArgs(args.Units, charmUUID)
	if err != nil {
		return errors.Errorf("creating unit args: %w", err)
	}

	if modelType == model.IAAS {
		err = s.st.InsertMigratingIAASUnits(ctx, appID, unitArgs...)
	} else {
		err = s.st.InsertMigratingCAASUnits(ctx, appID, unitArgs...)
	}
	if err != nil {
		return errors.Errorf("inserting units for application %q: %w", name, err)
	}

	if err := s.st.SetDesiredApplicationScale(ctx, appID, args.ScaleState.Scale); err != nil {
		return errors.Errorf("setting desired scale for application %q: %w", name, err)
	}
	if err := s.st.SetApplicationScalingState(ctx, name, args.ScaleState.ScaleTarget, args.ScaleState.Scaling); err != nil {
		return errors.Errorf("setting scale state for application %q: %w", name, err)
	}

	if err := s.st.SetApplicationConstraints(ctx, appID, constraints.DecodeConstraints(args.ApplicationConstraints)); err != nil {
		return errors.Errorf("setting application constraints for application %q: %w", name, err)
	}

	return nil
}

func makeUnitArgs(units []ImportUnitArg, charmUUID corecharm.ID) ([]application.InsertUnitArg, error) {
	unitArgs := make([]application.InsertUnitArg, len(units))
	for i, u := range units {

		arg := application.InsertUnitArg{
			UnitName:         u.UnitName,
			CharmUUID:        charmUUID,
			StorageParentDir: application.StorageParentDir,
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

// RemoveImportedApplication removes an application that was imported. The
// application might be in an incomplete state, so it's important to remove
// as much of the application as possible, even on failure.
func (s *MigrationService) RemoveImportedApplication(context.Context, string) error {
	// TODO (stickupkid): This is a placeholder for now, we need to implement
	// this method.
	return nil
}
