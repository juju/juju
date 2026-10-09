// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"math"
	"strconv"
	"strings"

	"github.com/juju/clock"

	corecharm "github.com/juju/juju/core/charm"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/ipaddress"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

// MigrationState is the state required for migrating applications.
type MigrationState interface {
	// GetApplicationUnitSequence returns the imported unit ordinal high-water
	// mark, or false when the sequence is absent.
	GetApplicationUnitSequence(context.Context, string) (uint64, bool, error)
	// EnsureApplicationUnitSequenceAtLeast advances that high-water mark.
	EnsureApplicationUnitSequenceAtLeast(context.Context, string, uint64) error
	// GetSpaceUUIDByName returns the UUID of the space with the given name.
	// It returns an error satisfying [networkerrors.SpaceNotFound] if the provided
	//
	// space name doesn't exist.
	GetSpaceUUIDByName(ctx context.Context, name string) (network.SpaceUUID, error)

	// InsertMigratingApplication inserts a migrating application. Returns as
	// error satisfying [applicationerrors.ApplicationAlreadyExists] if the
	// application already exists. If returns as error satisfying
	// [applicationerrors.CharmNotFound] if the charm for the application is
	// not found.
	InsertMigratingApplication(context.Context, string, application.InsertApplicationArgs) error
}

// MigrationService provides the API for migrating applications.
type MigrationService struct {
	st     State
	clock  clock.Clock
	logger logger.Logger
}

// NewMigrationService returns a new service reference wrapping the input state.
func NewMigrationService(
	st State,
	clock clock.Clock,
	logger logger.Logger,
) *MigrationService {
	return &MigrationService{
		st:     st,
		clock:  clock,
		logger: logger,
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

	if !application.IsValidCharmName(args.Name) {
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

	if !application.IsValidApplicationName(name) {
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

	config, err := application.DecodeConfig(ch.Config)
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
	), locator, nil
}

// GetApplicationCharmOrigin returns the charm origin for the specified
// application name. If the application does not exist, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *MigrationService) GetApplicationCharmOrigin(ctx context.Context, name string) (application.CharmOrigin, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !application.IsValidApplicationName(name) {
		return application.CharmOrigin{}, applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationUUIDByName(ctx, name)
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
func (s *MigrationService) GetApplicationConfigAndSettings(ctx context.Context, name string) (internalcharm.Config, application.ApplicationSettings, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !application.IsValidApplicationName(name) {
		return nil, application.ApplicationSettings{}, applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationUUIDByName(ctx, name)
	if err != nil {
		return nil, application.ApplicationSettings{}, errors.Capture(err)
	}

	cfg, settings, err := s.st.GetApplicationConfigAndSettings(ctx, appID)
	if err != nil {
		return nil, application.ApplicationSettings{}, errors.Capture(err)
	}

	appConfig, err := decodeApplicationConfig(cfg)
	if err != nil {
		return nil, application.ApplicationSettings{}, errors.Errorf("decoding application config: %w", err)
	}

	return appConfig, settings, nil
}

// GetApplicationConstraints returns the application constraints for the
// specified application name.
// Empty constraints are returned if no constraints exist for the given
// application UUID.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *MigrationService) GetApplicationConstraints(ctx context.Context, name string) (coreconstraints.Value, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !application.IsValidApplicationName(name) {
		return coreconstraints.Value{}, applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationUUIDByName(ctx, name)
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

	if !application.IsValidApplicationName(name) {
		return application.ScaleState{}, applicationerrors.ApplicationNameNotValid
	}

	appID, err := s.st.GetApplicationUUIDByName(ctx, name)
	if err != nil {
		return application.ScaleState{}, errors.Capture(err)
	}

	return s.st.GetApplicationScaleState(ctx, appID)
}

// ImportCAASApplication imports the specified CAAS application and units
// if required, returning an error satisfying
// [applicationerrors.ApplicationAlreadyExists] if the application already
// exists.
func (s *MigrationService) ImportCAASApplication(ctx context.Context, name string, args ImportCAASApplicationArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	charmUUID, err := s.importCAASApplication(ctx, name, args)
	if err != nil {
		return errors.Errorf("importing application %q: %w", name, err)
	}

	// TODO hml 1-May-25
	// Improve the efficiency of importing caas applications by touching
	// the application_scale table once, instead of three times. Once in
	// st.ImportApplication and the following two methods.
	if err := s.st.SetApplicationScalingStateWithStart(
		ctx, name, args.ScaleState.ScaleTarget, args.ScaleState.StartOrdinal,
		args.ScaleState.Scaling); err != nil {
		return errors.Errorf("setting scale state for application %q: %w", name, err)
	}
	if err := s.st.SetDesiredApplicationScale(ctx, args.UUID, args.ScaleState.Scale); err != nil {
		return errors.Errorf("setting desired scale for application %q: %w", name, err)
	}

	unitArgs, err := makeCAASUnitArgs(args.Units, charmUUID)
	if err != nil {
		return errors.Errorf("creating unit args: %w", err)
	}

	if err := s.st.InsertMigratingCAASUnits(ctx, args.UUID, unitArgs...); err != nil {
		return errors.Capture(err)
	}
	return s.ReconcileImportedCAASUnits(ctx, name, false)
}

// ReconcileImportedCAASUnits restores pending unit identities and the ordinal
// high-water mark before a migrated Kubernetes application can be provisioned.
// explicitStart is true for same-level imports, which carry start_ordinal.
func (s *MigrationService) ReconcileImportedCAASUnits(ctx context.Context, name string, explicitStart bool) error {
	appUUID, err := s.st.GetApplicationUUIDByName(ctx, name)
	if err != nil {
		return errors.Capture(err)
	}
	scaleState, err := s.st.GetApplicationScaleState(ctx, appUUID)
	if err != nil {
		return errors.Capture(err)
	}
	unitLives, err := s.st.GetAllUnitLifeForApplication(ctx, appUUID)
	if err != nil {
		return errors.Capture(err)
	}
	podIDs, err := s.st.GetAllUnitK8sPodIDsForApplication(ctx, appUUID)
	if err != nil {
		return errors.Capture(err)
	}
	last, hasSequence, err := s.st.GetApplicationUnitSequence(ctx, name)
	if err != nil {
		return errors.Capture(err)
	}

	aliveMin, aliveMax, aliveCount := -1, -1, 0
	for unitName, unitLife := range unitLives {
		parsed, err := coreunit.NewName(unitName)
		if err != nil || parsed.Application() != name {
			return errors.Errorf("invalid imported unit name %q for application %q", unitName, name)
		}
		ordinal := parsed.Number()
		if !hasSequence || uint64(ordinal) > last {
			last, hasSequence = uint64(ordinal), true
		}
		if unitLife == int(domainlife.Alive) {
			aliveCount++
			if aliveMin < 0 || ordinal < aliveMin {
				aliveMin = ordinal
			}
			if ordinal > aliveMax {
				aliveMax = ordinal
			}
		}
	}
	for _, providerID := range podIDs {
		prefix, ordinalText, ok := strings.Cut(providerID, name+"-")
		if !ok || prefix != "" {
			continue
		}
		ordinal, err := strconv.Atoi(ordinalText)
		if err != nil || ordinal < 0 {
			continue
		}
		if !hasSequence || uint64(ordinal) > last {
			last, hasSequence = uint64(ordinal), true
		}
	}

	target := scaleState.Scale
	if scaleState.Scaling {
		target = scaleState.ScaleTarget
	}
	if target < 0 || scaleState.StartOrdinal < 0 {
		return errors.Errorf("invalid imported scale range for application %q", name)
	}
	start := scaleState.StartOrdinal
	if aliveMin >= 0 && !explicitStart {
		start = aliveMin
	} else if aliveMin < 0 && hasSequence && target == 0 {
		if last >= math.MaxInt {
			return errors.Errorf("unit sequence exhausted for application %q", name)
		}
		if start <= int(last) {
			start = int(last) + 1
		}
	} else if aliveMin < 0 && hasSequence && !explicitStart && target > 0 &&
		last >= uint64(target-1) {
		// Legacy exports have no start ordinal. With no registered units,
		// recover the most recent target range from the sequence.
		start = int(last) - target + 1
	}
	if target > math.MaxInt-start {
		return errors.Errorf("imported scale range overflows for application %q", name)
	}
	if target > 0 {
		floor := uint64(start + target - 1)
		if !hasSequence || floor > last {
			last, hasSequence = floor, true
		}
	} else if scaleState.StartOrdinal > 0 {
		floor := uint64(scaleState.StartOrdinal - 1)
		if !hasSequence || floor > last {
			last, hasSequence = floor, true
		}
	}
	if hasSequence {
		if err := s.st.EnsureApplicationUnitSequenceAtLeast(ctx, name, last); err != nil {
			return errors.Errorf("reconciling unit sequence for application %q: %w", name, err)
		}
	}
	if start != scaleState.StartOrdinal {
		if err := s.st.SetApplicationScalingStateWithStart(
			ctx, name, scaleState.ScaleTarget, start, scaleState.Scaling,
		); err != nil {
			return errors.Errorf("restoring ordinal range for application %q: %w", name, err)
		}
	}

	appLife, err := s.st.GetApplicationLife(ctx, appUUID)
	if err != nil {
		return errors.Capture(err)
	}
	if appLife != domainlife.Alive || target == 0 || aliveCount > target ||
		(aliveMin >= 0 && aliveMax-aliveMin+1 > target) {
		return nil
	}
	now := new(s.clock.Now().UTC())
	reservations := make([]application.AddCAASUnitArg, 0, target-aliveCount)
	for ordinal := start; ordinal < start+target; ordinal++ {
		unitName, err := coreunit.NewNameFromParts(name, ordinal)
		if err != nil {
			return errors.Capture(err)
		}
		if unitLife, exists := unitLives[unitName.String()]; exists {
			if unitLife != int(domainlife.Alive) {
				return errors.Errorf("imported unit %q is leaving inside the requested range", unitName)
			}
			continue
		}
		unitUUID, err := coreunit.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		netNodeUUID, err := domainnetwork.NewNetNodeUUID()
		if err != nil {
			return errors.Capture(err)
		}
		reservations = append(reservations, application.AddCAASUnitArg{
			AddUnitArg: application.AddUnitArg{
				UnitUUID:    unitUUID,
				NetNodeUUID: netNodeUUID,
				UnitStatusArg: application.UnitStatusArg{
					AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
						Status: status.UnitAgentStatusAllocating, Since: now,
					},
					WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
						Status: status.WorkloadStatusWaiting, Message: corestatus.MessageInstallingAgent, Since: now,
					},
				},
			},
			ReservedName: unitName,
		})
	}
	if len(reservations) == 0 {
		return nil
	}
	if _, err := s.st.AddCAASUnits(ctx, appUUID, reservations...); err != nil {
		return errors.Errorf("materialising imported units for application %q: %w", name, err)
	}
	return nil
}

// ImportIAASApplication imports the specified IAAS application and units
// if required, returning an error satisfying
// [applicationerrors.ApplicationAlreadyExists] if the application already
// exists.
func (s *MigrationService) ImportIAASApplication(ctx context.Context, name string, args ImportIAASApplicationArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	charmUUID, err := s.importIAASApplication(ctx, name, args)
	if err != nil {
		return errors.Errorf("importing application %q: %w", name, err)
	}

	unitArgs, err := makeIAASUnitArgs(args.Units, charmUUID)
	if err != nil {
		return errors.Errorf("creating unit args: %w", err)
	}

	return s.st.InsertMigratingIAASUnits(ctx, args.UUID, unitArgs...)
}

func (s *MigrationService) importIAASApplication(
	ctx context.Context,
	name string,
	args ImportIAASApplicationArgs,
) (corecharm.ID, error) {
	if err := validateCharmAndApplicationParams(name, args.ReferenceName, args.Charm, args.CharmOrigin); err != nil {
		return "", errors.Errorf("invalid application args: %w", err)
	}

	appArg, err := makeInsertApplicationArg(args.ImportApplicationArgs)
	if err != nil {
		return "", errors.Errorf("creating application args: %w", err)
	}

	if err := s.st.InsertMigratingApplication(ctx, name, appArg); err != nil {
		return "", errors.Errorf("creating application %q: %w", name, err)
	}

	charmUUID, err := s.st.GetCharmIDByApplicationName(ctx, name)
	if err != nil {
		return "", errors.Errorf("getting charm ID for application %q: %w", name, err)
	}

	if err := s.st.MergeExposeSettings(ctx, args.UUID, args.ExposedEndpoints); err != nil {
		return "", errors.Errorf("setting expose settings for application %q: %w", name, err)
	}
	if err := s.st.SetApplicationConstraints(ctx, args.UUID, constraints.DecodeConstraints(args.ApplicationConstraints)); err != nil {
		return "", errors.Errorf("setting application constraints for application %q: %w", name, err)
	}

	return charmUUID, nil
}

func (s *MigrationService) importCAASApplication(
	ctx context.Context,
	name string,
	args ImportCAASApplicationArgs,
) (corecharm.ID, error) {
	if err := validateCharmAndApplicationParams(name, args.ReferenceName, args.Charm, args.CharmOrigin); err != nil {
		return "", errors.Errorf("invalid application args: %w", err)
	}

	appArg, err := makeInsertApplicationArg(args.ImportApplicationArgs)
	if err != nil {
		return "", errors.Errorf("creating application args: %w", err)
	}

	appArg.Scale = len(args.Units)

	if err := s.st.InsertMigratingApplication(ctx, name, appArg); err != nil {
		return "", errors.Errorf("creating application %q: %w", name, err)
	}

	charmUUID, err := s.st.GetCharmIDByApplicationName(ctx, name)
	if err != nil {
		return "", errors.Errorf("getting charm ID for application %q: %w", name, err)
	}

	if err := s.st.MergeExposeSettings(ctx, args.UUID, args.ExposedEndpoints); err != nil {
		return "", errors.Errorf("setting expose settings for application %q: %w", name, err)
	}
	if err := s.st.SetApplicationConstraints(ctx, args.UUID, constraints.DecodeConstraints(args.ApplicationConstraints)); err != nil {
		return "", errors.Errorf("setting application constraints for application %q: %w", name, err)
	}

	return charmUUID, nil
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

	applicationConfig, err := application.EncodeApplicationConfig(args.ApplicationConfig, ch.Config)
	if err != nil {
		return application.InsertApplicationArgs{}, errors.Errorf("encoding application config: %w", err)
	}

	storageDirectives, err := makeMigratingStorageDirectiveArgs(args.StorageDirectives)
	if err != nil {
		return application.InsertApplicationArgs{}, errors.Errorf("encoding storage directives: %w", err)
	}

	return application.InsertApplicationArgs{
		ApplicationUUID:   args.UUID.String(),
		Charm:             ch,
		Platform:          platformArg,
		Channel:           channelArg,
		EndpointBindings:  args.EndpointBindings,
		Resources:         makeResourcesArgs(args.ResolvedResources),
		Config:            applicationConfig,
		Settings:          args.ApplicationSettings,
		StorageDirectives: storageDirectives,
	}, nil
}

// makeMigratingStorageDirectiveArgs converts the imported storage directive
// arguments into the migrating storage directive arguments understood by the
// state layer. The storage pool is left as a name; the state layer resolves it
// to a storage pool UUID.
func makeMigratingStorageDirectiveArgs(
	directives []ImportStorageDirectiveArg,
) ([]application.MigratingStorageDirectiveArg, error) {
	if len(directives) == 0 {
		return nil, nil
	}
	result := make([]application.MigratingStorageDirectiveArg, 0, len(directives))
	for _, d := range directives {
		result = append(result, application.MigratingStorageDirectiveArg{
			Name:     d.Name,
			PoolName: d.Pool,
			Size:     d.Size,
			Count:    uint32(d.Count),
		})
	}
	return result, nil
}

// IsApplicationExposed returns whether the provided application is exposed or not.
//
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *MigrationService) IsApplicationExposed(ctx context.Context, appName string) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, err := s.st.GetApplicationUUIDByName(ctx, appName)
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

	appID, err := s.st.GetApplicationUUIDByName(ctx, appName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return s.st.GetExposedEndpoints(ctx, appID)
}

// GetSpaceUUIDByName returns the UUID of the space with the given name.
//
// It returns an error satisfying [networkerrors.SpaceNotFound] if the provided
// space name doesn't exist.
func (s *MigrationService) GetSpaceUUIDByName(ctx context.Context, name string) (network.SpaceUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetSpaceUUIDByName(ctx, name)
}

func makeIAASUnitArgs(units []ImportIAASUnitArg, charmUUID corecharm.ID) ([]application.ImportIAASUnitArg, error) {
	unitArgs := make([]application.ImportIAASUnitArg, len(units))
	for i, u := range units {
		arg := application.ImportUnitArg{
			UnitName:        u.UnitName,
			WorkloadVersion: u.WorkloadVersion,
		}
		if u.PasswordHash != nil {
			arg.Password = &application.PasswordInfo{
				PasswordHash:  *u.PasswordHash,
				HashAlgorithm: application.HashAlgorithmSHA256,
			}
		}
		unitArgs[i] = application.ImportIAASUnitArg{
			ImportUnitArg: arg,
			Principal:     u.Principal,
			Machine:       u.Machine,
		}
	}
	return unitArgs, nil
}

func makeCAASUnitArgs(units []ImportCAASUnitArg, charmUUID corecharm.ID) ([]application.ImportCAASUnitArg, error) {
	unitArgs := make([]application.ImportCAASUnitArg, len(units))
	for i, u := range units {
		arg := application.ImportUnitArg{
			UnitName:        u.UnitName,
			WorkloadVersion: u.WorkloadVersion,
		}

		if u.PasswordHash != nil {
			arg.Password = &application.PasswordInfo{
				PasswordHash:  *u.PasswordHash,
				HashAlgorithm: application.HashAlgorithmSHA256,
			}
		}

		var k8sPod *application.K8sPod
		if u.K8sPod != nil {
			k8sPod = makeK8sPodArg(*u.K8sPod)
		}

		unitArgs[i] = application.ImportCAASUnitArg{
			ImportUnitArg: arg,
			K8sPod:        k8sPod,
		}
	}
	return unitArgs, nil
}

func makeK8sPodArg(k8sPod application.K8sPodParams) *application.K8sPod {
	result := &application.K8sPod{
		ProviderID: k8sPod.ProviderID,
		Ports:      k8sPod.Ports,
	}
	if k8sPod.Address != nil {
		// TODO(units) - handle the k8sPod.Address space ID
		// For k8s we'll initially create a /32 subnet off the container address
		// and add that to the default space.
		result.Address = &application.K8sPodAddress{
			// For k8s pods, the device is a placeholder without
			// a MAC address and once inserted, not updated. It just exists
			// to tie the address to the net node corresponding to the
			// k8s pod.
			Device: application.K8sPodDevice{
				Name:              network.PlaceholderDeviceName,
				DeviceTypeID:      domainnetwork.DeviceTypeUnknown,
				VirtualPortTypeID: domainnetwork.NonVirtualPortType,
			},
			Value:       k8sPod.Address.Value,
			AddressType: ipaddress.MarshallAddressType(k8sPod.Address.AddressType()),
			Scope:       ipaddress.MarshallScope(k8sPod.Address.Scope),
			Origin:      ipaddress.MarshallOrigin(network.OriginProvider),
			ConfigType:  ipaddress.MarshallConfigType(network.ConfigDHCP),
		}
		if k8sPod.AddressOrigin != nil {
			result.Address.Origin = ipaddress.MarshallOrigin(*k8sPod.AddressOrigin)
		}
	}
	return result
}
