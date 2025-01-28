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
	"github.com/juju/juju/core/logger"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
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

// GetCharm returns the charm using the charm ID.
// Calling this method will return all the data associated with the charm.
// It is not expected to call this method for all calls, instead use the move
// focused and specific methods. That's because this method is very expensive
// to call. This is implemented for the cases where all the charm data is
// needed; model migration, charm export, etc.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *MigrationService) GetCharm(ctx context.Context, id corecharm.ID) (internalcharm.Charm, charm.CharmLocator, error) {
	if err := id.Validate(); err != nil {
		return nil, charm.CharmLocator{}, fmt.Errorf("charm id: %w", err)
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

// GetApplicationConfig returns the application config for the specified
// application. This will return the application config and the settings in
// one config.ConfigAttributes object.
//
// If the application does not exist, a [applicationerrors.ApplicationNotFound]
// error is returned.
// If no config is set for the application, an empty config is returned.
func (s *MigrationService) GetApplicationConfig(ctx context.Context, name string) (config.ConfigAttributes, application.ApplicationSettings, error) {
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

// ImportApplication imports the specified application and units if required,
// returning an error satisfying [applicationerrors.ApplicationAlreadyExists]
// if the application already exists.
func (s *MigrationService) ImportApplication(ctx context.Context, name string, args ImportApplicationArgs) error {
	if err := validateCreateApplicationParams(name, args.ReferenceName, args.Charm, args.CharmOrigin, args.DownloadInfo, args.ResolvedResources, s.logger); err != nil {
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
	})
	if err != nil {
		return errors.Annotatef(err, "creating application args")
	}

	units := args.Units
	numUnits := len(units)
	appArg.Scale = numUnits

	unitArgs := make([]application.InsertUnitArg, numUnits)
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
		appID, err := s.st.CreateApplication(ctx, name, appArg)
		if err != nil {
			return errors.Annotatef(err, "creating application %q", name)
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

// RemoveImportedApplication removes an application that was imported. The
// application might be in an incomplete state, so it's important to remove
// as much of the application as possible, even on failure.
func (s *MigrationService) RemoveImportedApplication(context.Context, string) error {
	// TODO (stickupkid): This is a placeholder for now, we need to implement
	// this method.
	return nil
}

func (s *MigrationService) makeUnitStatus(in StatusParams) application.StatusInfo {
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
