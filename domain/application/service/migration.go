// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/errors"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	corestatus "github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/linklayerdevice"
	internalcharm "github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
)

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
		return nil, charm.CharmLocator{}, errors.Trace(err)
	}

	ch, _, err := s.st.GetCharm(ctx, id)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Trace(err)
	}

	// The charm needs to be decoded into the internalcharm.Charm type.

	metadata, err := decodeMetadata(ch.Metadata)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Trace(err)
	}

	manifest, err := decodeManifest(ch.Manifest)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Trace(err)
	}

	actions, err := decodeActions(ch.Actions)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Trace(err)
	}

	config, err := decodeConfig(ch.Config)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Trace(err)
	}

	lxdProfile, err := decodeLXDProfile(ch.LXDProfile)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Trace(err)
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
		return nil, application.ApplicationSettings{}, errors.Trace(err)
	}

	cfg, settings, err := s.st.GetApplicationConfigAndSettings(ctx, appID)
	if err != nil {
		return nil, application.ApplicationSettings{}, errors.Trace(err)
	}

	result := make(config.ConfigAttributes)
	for k, v := range cfg {
		result[k] = v.Value
	}
	return result, settings, nil
}

// GetApplicationStatus returns the status of the specified application.
// If the application does not exist, a [applicationerrors.ApplicationNotFound]
// error is returned.
func (s *MigrationService) GetApplicationStatus(ctx context.Context, name string) (*corestatus.StatusInfo, error) {
	if !isValidApplicationName(name) {
		return nil, applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationIDByName(ctx, name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	status, err := s.st.GetApplicationStatus(ctx, appID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	decodedStatus, err := decodeWorkloadStatus(status)
	if err != nil {
		return nil, errors.Annotatef(err, "decoding workload status")
	}
	return decodedStatus, nil
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
		return coreconstraints.Value{}, errors.Trace(err)
	}

	cons, err := s.st.GetApplicationConstraints(ctx, appID)
	return constraints.EncodeConstraints(cons), internalerrors.Capture(err)
}

// GetUnitWorkloadStatus returns the workload status of the specified unit, returning an
// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (s *MigrationService) GetUnitWorkloadStatus(ctx context.Context, unitName coreunit.Name) (*corestatus.StatusInfo, error) {
	if err := unitName.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	workloadStatus, err := s.st.GetUnitWorkloadStatus(ctx, unitUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return decodeWorkloadStatus(workloadStatus)
}

// ImportApplication imports the specified application and units if required,
// returning an error satisfying [applicationerrors.ApplicationAlreadyExists]
// if the application already exists.
func (s *MigrationService) ImportApplication(ctx context.Context, name string, args ImportApplicationArgs) error {
	if err := validateCharmAndApplicationParams(name, args.ReferenceName, args.Charm, args.CharmOrigin, args.DownloadInfo); err != nil {
		return errors.Annotatef(err, "invalid application args")
	}

	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return errors.Annotatef(err, "getting model type")
	}
	appArg, err := makeCreateApplicationArgs(ctx, s.st, s.storageRegistryGetter, modelType, args.Charm, args.CharmOrigin, AddApplicationArgs{
		ReferenceName:       args.ReferenceName,
		DownloadInfo:        args.DownloadInfo,
		ApplicationConfig:   args.ApplicationConfig,
		ApplicationSettings: args.ApplicationSettings,
		ApplicationStatus:   args.ApplicationStatus,
	})
	if err != nil {
		return errors.Annotatef(err, "creating application args")
	}

	units := args.Units
	numUnits := len(units)
	appArg.Scale = numUnits

	unitArgs := make([]application.InsertUnitArg, numUnits)
	for i, u := range units {
		agentStatus, err := encodeUnitAgentStatus(&u.AgentStatus)
		if err != nil {
			return errors.Annotatef(err, "encoding agent status for unit %q", u.UnitName)
		}
		workloadStatus, err := encodeWorkloadStatus(&u.WorkloadStatus)
		if err != nil {
			return errors.Annotatef(err, "encoding workload status for unit %q", u.UnitName)
		}

		arg := application.InsertUnitArg{
			UnitName: u.UnitName,
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus:    agentStatus,
				WorkloadStatus: workloadStatus,
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

	appID, err := s.st.CreateApplication(ctx, name, appArg, nil)
	if err != nil {
		return errors.Annotatef(err, "creating application %q", name)
	}
	for _, arg := range unitArgs {
		if err := s.st.InsertUnit(ctx, appID, arg); err != nil {
			return errors.Annotatef(err, "inserting unit %q", arg.UnitName)
		}
	}
	// TODO(nvinuesa): We should use the service method to set the
	// constraints because we need validation. For the moment it's not
	// possible because we need a provider during migration.
	err = s.st.SetApplicationConstraints(ctx, appID, constraints.DecodeConstraints(args.ApplicationConstraints))
	if err != nil {
		return errors.Annotatef(err, "setting application constraints for application %q", name)
	}

	return nil
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
