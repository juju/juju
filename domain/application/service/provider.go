// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strconv"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/assumes"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	coreconstraints "github.com/juju/juju/core/constraints"
	corecontroller "github.com/juju/juju/core/controller"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/status"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/environs"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/storage"
)

// Provider defines the interface for interacting with the underlying model
// provider.
type Provider interface {
	environs.ConstraintsChecker
	environs.InstancePrechecker
}

// CAASProvider defines the interface for interacting with the
// underlying provider for CAAS applications.
type CAASProvider interface {
	environs.SupportedFeatureEnumerator
	Application(string, caas.DeploymentType) caas.Application
}

// ProviderService defines a service for interacting with the underlying
// model state.
type ProviderService struct {
	*Service

	modelID                 coremodel.UUID
	agentVersionGetter      AgentVersionGetter
	provider                providertracker.ProviderGetter[Provider]
	caasApplicationProvider providertracker.ProviderGetter[CAASProvider]
}

// NewProviderService returns a new Service for interacting with a models state.
func NewProviderService(
	st State,
	leaderEnsurer leadership.Ensurer,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	modelID coremodel.UUID,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
	caasApplicationProvider providertracker.ProviderGetter[CAASProvider],
	charmStore CharmStore,
	statusHistory StatusHistory,
	clock clock.Clock,
	logger logger.Logger,
) *ProviderService {
	return &ProviderService{
		Service: NewService(
			st,
			leaderEnsurer,
			storageRegistryGetter,
			charmStore,
			statusHistory,
			clock,
			logger,
		),
		modelID:                 modelID,
		agentVersionGetter:      agentVersionGetter,
		provider:                provider,
		caasApplicationProvider: caasApplicationProvider,
	}
}

// CreateIAASApplication creates the specified IAAS application and units if
// required, returning an error satisfying
// [applicationerrors.ApplicationAlreadyExists] if the application already
// exists.
func (s *ProviderService) CreateIAASApplication(
	ctx context.Context,
	name string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
	units ...AddIAASUnitArg,
) (coreapplication.ID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appName, appArg, unitArgs, err := s.makeIAASApplicationArg(ctx, name, charm, origin, args, units...)
	if err != nil {
		return "", errors.Errorf("preparing IAAS application args: %w", err)
	}

	// Precheck any instances that are being created.
	if err := s.precheckInstances(ctx, appArg.Platform, transform.Slice(unitArgs, func(arg application.AddIAASUnitArg) application.AddUnitArg {
		return arg.AddUnitArg
	})); err != nil {
		return "", errors.Errorf("prechecking instances: %w", err)
	}

	appID, machineNames, err := s.st.CreateIAASApplication(ctx, appName, appArg, unitArgs)
	if err != nil {
		return "", errors.Errorf("creating IAAS application %q: %w", appName, err)
	}

	s.logger.Infof(ctx, "created IAAS application %q with ID %q", appName, appID)

	if args.ApplicationStatus != nil {
		if err := s.statusHistory.RecordStatus(ctx, status.ApplicationNamespace.WithID(appName), *args.ApplicationStatus); err != nil {
			s.logger.Infof(ctx, "failed recording IAAS application status history: %w", err)
		}
	}
	s.recordInitMachinesStatusHistory(ctx, machineNames)

	return appID, nil
}

// CreateCAASApplication creates the specified CAAS application and units if
// required, returning an error satisfying
// [applicationerrors.ApplicationAlreadyExists] if the application already
// exists.
func (s *ProviderService) CreateCAASApplication(
	ctx context.Context,
	name string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
	units ...AddUnitArg,
) (coreapplication.ID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appName, appArg, unitArgs, err := s.makeCAASApplicationArg(ctx, name, charm, origin, args, units...)
	if err != nil {
		return "", errors.Errorf("preparing CAAS application args: %w", err)
	}

	// Precheck any instances that are being created.
	if err := s.precheckInstances(ctx, appArg.Platform, unitArgs); err != nil {
		return "", errors.Errorf("prechecking instances: %w", err)
	}

	appID, err := s.st.CreateCAASApplication(ctx, appName, appArg, unitArgs)
	if err != nil {
		return "", errors.Errorf("creating CAAS application %q: %w", appName, err)
	}

	s.logger.Infof(ctx, "created CAAS application %q with ID %q", appName, appID)

	if args.ApplicationStatus != nil {
		if err := s.statusHistory.RecordStatus(ctx, status.ApplicationNamespace.WithID(appName), *args.ApplicationStatus); err != nil {
			s.logger.Infof(ctx, "failed recording CAAS application status history: %w", err)
		}
	}

	return appID, nil
}

// GetSupportedFeatures returns the set of features that the model makes
// available for charms to use.
// If the agent version cannot be found, an error satisfying
// [modelerrors.NotFound] will be returned.
func (s *ProviderService) GetSupportedFeatures(ctx context.Context) (assumes.FeatureSet, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	agentVersion, err := s.agentVersionGetter.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return assumes.FeatureSet{}, err
	}

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{
		Name:        "juju",
		Description: assumes.UserFriendlyFeatureDescriptions["juju"],
		Version:     &agentVersion,
	})

	supportedFeatureProvider, err := s.caasApplicationProvider(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return fs, nil
	} else if err != nil {
		return fs, err
	}

	envFs, err := supportedFeatureProvider.SupportedFeatures()
	if err != nil {
		return fs, errors.Errorf("enumerating features supported by environment: %w", err)
	}

	fs.Merge(envFs)

	return fs, nil
}

// SetApplicationConstraints sets the application constraints for the
// specified application ID.
// This method overwrites the full constraints on every call.
// If invalid constraints are provided (e.g. invalid container type or
// non-existing space), a [applicationerrors.InvalidApplicationConstraints]
// error is returned.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *ProviderService) SetApplicationConstraints(ctx context.Context, appID coreapplication.ID, cons coreconstraints.Value) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return errors.Errorf("application ID: %w", err)
	}
	if err := s.validateConstraints(ctx, cons); err != nil {
		return err
	}

	return s.st.SetApplicationConstraints(ctx, appID, constraints.DecodeConstraints(cons))
}

// AddControllerIAASUnits adds the specified number of controller units to
// the controller application. It expects to only operate on the controller
// model, thus there should always be a controller application.
// This is additive, meaning that it will never remove any existing
// controllers.
func (s *ProviderService) AddControllerIAASUnits(
	ctx context.Context,
	controllerIDs []string,
	units []AddIAASUnitArg,
) ([]coremachine.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// This can happen if we try and enable HA before the first controller
	// machine is created and assigned a address.
	if len(controllerIDs) == 0 {
		return nil, errors.New("no existing controller IDs provided")
	}

	newUnits := len(units) + len(controllerIDs)
	if newUnits != 0 && newUnits%2 != 1 {
		return nil, errors.New("number of controllers must be odd and non-negative")
	} else if newUnits > corecontroller.MaxPeers {
		return nil, errors.Errorf("controller count is too large (allowed %d)", corecontroller.MaxPeers)
	}

	for _, unit := range units {
		if unit.Placement == nil {
			continue
		}

		// Machines cannot be placed on containers, so we check if the
		// placement is a container machine and if so, return an error.
		if unit.Placement.Scope == instance.MachineScope && names.IsContainerMachine(unit.Placement.Directive) {
			return nil, errors.Errorf("controller units cannot be placed on containers").Add(coreerrors.NotSupported)
		}

		// Machines cannot be co-located on existing controller machines,
		if controller, err := s.st.IsMachineController(ctx, coremachine.Name(unit.Placement.Directive)); err != nil {
			return nil, errors.Errorf("checking if machine %q is a controller: %w", unit.Placement.Directive, err)
		} else if controller {
			return nil, errors.Errorf("controller units cannot be placed on controller machines %q", unit.Placement.Directive)
		}
	}

	// Calculate the offset for the new controllers. This is the new number
	// minus the number of existing controllers.
	required := len(units) - len(controllerIDs)
	if required == 0 {
		// No changes are required, so we return an empty result.
		return nil, nil
	}

	// Add the required number of units to the controller application.
	_, machineNames, err := s.AddIAASUnits(ctx, coreapplication.ControllerApplicationName, units...)
	if err != nil {
		return nil, errors.Errorf("adding IAAS units to controller application: %w", err)
	}
	return machineNames, nil
}

// AddIAASUnits adds the specified units to the IAAS application, returning an
// error satisfying [applicationerrors.ApplicationNotFoundError] if the
// application doesn't exist. If no units are provided, it will return nil.
func (s *ProviderService) AddIAASUnits(ctx context.Context, appName string, units ...AddIAASUnitArg) ([]coreunit.Name, []coremachine.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(units) == 0 {
		return nil, nil, nil
	}

	if !isValidApplicationName(appName) {
		return nil, nil, applicationerrors.ApplicationNameNotValid
	}

	appUUID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return nil, nil, errors.Errorf("getting application %q id: %w", appName, err)
	}

	cons, err := s.makeApplicationConstraints(ctx, appUUID)
	if err != nil {
		return nil, nil, errors.Errorf("making application %q constraints: %w", appName, err)
	}

	origin, err := s.st.GetApplicationCharmOrigin(ctx, appUUID)
	if err != nil {
		return nil, nil, errors.Errorf("getting application %q platform: %w", appName, err)
	}

	args, err := s.makeIAASUnitArgs(units, origin.Platform, constraints.DecodeConstraints(cons))
	if err != nil {
		return nil, nil, errors.Errorf("making IAAS unit args: %w", err)
	}

	if err := s.precheckInstances(ctx, origin.Platform, transform.Slice(args, func(arg application.AddIAASUnitArg) application.AddUnitArg {
		return arg.AddUnitArg
	})); err != nil {
		return nil, nil, errors.Errorf("pre-checking instances: %w", err)
	}

	unitNames, machineNames, err := s.st.AddIAASUnits(ctx, appUUID, args...)
	if err != nil {
		return nil, nil, errors.Errorf("adding IAAS units to application %q: %w", appName, err)
	}

	for i, name := range unitNames {
		arg := args[i]
		if err := s.recordUnitStatusHistory(ctx, name, arg.UnitStatusArg); err != nil {
			return nil, nil, errors.Errorf("recording status history: %w", err)
		}
	}
	s.recordInitMachinesStatusHistory(ctx, machineNames)

	return unitNames, machineNames, nil
}

// AddCAASUnits adds the specified units to the CAAS application, returning an
// error satisfying [applicationerrors.ApplicationNotFoundError] if the
// application doesn't exist. If no units are provided, it will return nil.
func (s *ProviderService) AddCAASUnits(ctx context.Context, appName string, units ...AddUnitArg) ([]coreunit.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(units) == 0 {
		return []coreunit.Name{}, nil
	}

	if !isValidApplicationName(appName) {
		return nil, applicationerrors.ApplicationNameNotValid
	}

	appUUID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return nil, errors.Errorf("getting application %q id: %w", appName, err)
	}

	cons, err := s.makeApplicationConstraints(ctx, appUUID)
	if err != nil {
		return nil, errors.Errorf("making application %q constraints: %w", appName, err)
	}

	args, err := s.makeCAASUnitArgs(units, constraints.DecodeConstraints(cons))
	if err != nil {
		return nil, errors.Errorf("making CAAS unit args: %w", err)
	}

	origin, err := s.st.GetApplicationCharmOrigin(ctx, appUUID)
	if err != nil {
		return nil, errors.Errorf("getting application platform: %w", err)
	}
	if err := s.precheckInstances(ctx, origin.Platform, args); err != nil {
		return nil, errors.Errorf("pre-checking instances: %w", err)
	}

	unitNames, err := s.st.AddCAASUnits(ctx, appUUID, args...)
	if err != nil {
		return nil, errors.Errorf("adding CAAS units to application %q: %w", appName, err)
	}

	for i, name := range unitNames {
		arg := args[i]
		if err := s.recordUnitStatusHistory(ctx, name, arg.UnitStatusArg); err != nil {
			return nil, errors.Errorf("recording status history: %w", err)
		}
	}

	return unitNames, nil
}

// CAASUnitTerminating should be called by the CAASUnitTerminationWorker when
// the agent receives a signal to exit. UnitTerminating will return how the
// agent should shutdown.
//
// We pass in a CAAS broker to get app details from the k8s cluster - we will
// probably make it a service attribute once more use cases emerge.
func (s *ProviderService) CAASUnitTerminating(ctx context.Context, unitNameStr string) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	unitName, err := coreunit.NewName(unitNameStr)
	if err != nil {
		return false, errors.Errorf("parsing unit name %q: %w", unitNameStr, err)
	}

	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return false, errors.Errorf("getting unit %q life: %w", unitNameStr, err)
	}
	if unitLife != life.Alive {
		return false, nil
	}

	appName := unitName.Application()
	unitNum := unitName.Number()

	caasApplicationProvider, err := s.caasApplicationProvider(ctx)
	if err != nil {
		return false, errors.Errorf("terminating k8s unit %s/%q: %w", appName, unitNum, err)
	}

	// We currently only support statefulset.
	restart := true
	caasApp := caasApplicationProvider.Application(appName, caas.DeploymentStateful)
	appState, err := caasApp.State()
	if err != nil {
		return false, errors.Capture(err)
	}
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return false, errors.Capture(err)
	}
	scaleInfo, err := s.st.GetApplicationScaleState(ctx, appID)
	if err != nil {
		return false, errors.Capture(err)
	}
	if unitNum >= scaleInfo.Scale || unitNum >= appState.DesiredReplicas {
		restart = false
	}
	return restart, nil
}

// RegisterCAASUnit creates or updates the specified application unit in a caas
// model, returning an error satisfying
// [applicationerrors.ApplicationNotFoundError] if the application doesn't
// exist. If the unit life is Dead, an error satisfying
// [applicationerrors.UnitAlreadyExists] is returned.
func (s *ProviderService) RegisterCAASUnit(
	ctx context.Context,
	params application.RegisterCAASUnitParams,
) (coreunit.Name, string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if params.ProviderID == "" {
		return "", "", errors.Errorf("provider id %w", coreerrors.NotValid)
	}

	pass, err := password.RandomPassword()
	if err != nil {
		return "", "", errors.Errorf("generating unit password: %w", err)
	}
	registerArgs := application.RegisterCAASUnitArg{
		ProviderID:   params.ProviderID,
		PasswordHash: password.AgentPasswordHash(pass),
	}

	// We don't support anything other that statefulsets.
	// So the pod name contains the unit number.
	appName := params.ApplicationName
	splitPodName := strings.Split(params.ProviderID, "-")
	ord, err := strconv.Atoi(splitPodName[len(splitPodName)-1])
	if err != nil {
		return "", "", errors.Errorf("parsing unit number from pod name %q: %w", params.ProviderID, err)
	}
	unitName, err := coreunit.NewNameFromParts(appName, ord)
	if err != nil {
		return "", "", errors.Capture(err)
	}
	registerArgs.UnitName = unitName
	registerArgs.OrderedId = ord
	registerArgs.OrderedScale = true

	// Find the pod/unit in the provider.
	caasApplicationProvider, err := s.caasApplicationProvider(ctx)
	if err != nil {
		return "", "", errors.Errorf("registering k8s units for application %q: %w", appName, err)
	}
	caasApp := caasApplicationProvider.Application(appName, caas.DeploymentStateful)
	pods, err := caasApp.Units()
	if err != nil {
		return "", "", errors.Errorf("finding k8s units for application %q: %w", appName, err)
	}
	var pod *caas.Unit
	for _, v := range pods {
		p := v
		if p.Id == params.ProviderID {
			pod = &p
			break
		}
	}
	if pod == nil {
		return "", "", errors.Errorf("pod %s in provider %w", params.ProviderID, coreerrors.NotFound)
	}

	if pod.Address != "" {
		registerArgs.Address = &pod.Address
	}
	if len(pod.Ports) != 0 {
		registerArgs.Ports = &pod.Ports
	}
	for _, fs := range pod.FilesystemInfo {
		registerArgs.ObservedAttachedVolumeIDs = append(registerArgs.ObservedAttachedVolumeIDs, fs.Volume.VolumeId)
	}

	err = s.st.RegisterCAASUnit(ctx, appName, registerArgs)
	if err != nil {
		return "", "", errors.Errorf("saving caas unit %q: %w", registerArgs.UnitName, err)
	}
	return unitName, pass, nil
}

func (s *ProviderService) makeIAASApplicationArg(ctx context.Context,
	name string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
	units ...AddIAASUnitArg,
) (string, application.AddIAASApplicationArg, []application.AddIAASUnitArg, error) {
	appName, arg, err := s.makeApplicationArg(ctx, name, charm, origin, args)
	if err != nil {
		return "", application.AddIAASApplicationArg{}, nil, errors.Errorf("preparing IAAS application args: %w", err)
	}

	cons, err := s.mergeApplicationAndModelConstraints(ctx, constraints.DecodeConstraints(args.Constraints), charm.Meta().Subordinate)
	if err != nil {
		return "", application.AddIAASApplicationArg{}, nil, errors.Errorf("merging IAAS application and model constraints: %w", err)
	}

	unitArgs, err := s.makeIAASUnitArgs(units, arg.Platform, constraints.DecodeConstraints(cons))
	if err != nil {
		return "", application.AddIAASApplicationArg{}, nil, errors.Errorf("making IAAS unit args: %w", err)
	}

	return appName, application.AddIAASApplicationArg{
		BaseAddApplicationArg: arg,
	}, unitArgs, nil
}

func (s *ProviderService) makeCAASApplicationArg(
	ctx context.Context,
	name string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
	units ...AddUnitArg,
) (string, application.AddCAASApplicationArg, []application.AddUnitArg, error) {
	appName, arg, err := s.makeApplicationArg(ctx, name, charm, origin, args)
	if err != nil {
		return "", application.AddCAASApplicationArg{}, nil, errors.Errorf("preparing CAAS application args: %w", err)
	}

	cons, err := s.mergeApplicationAndModelConstraints(ctx, constraints.DecodeConstraints(args.Constraints), charm.Meta().Subordinate)
	if err != nil {
		return "", application.AddCAASApplicationArg{}, nil, errors.Errorf("merging CAAS application and model constraints: %w", err)
	}

	unitArgs, err := s.makeCAASUnitArgs(units, constraints.DecodeConstraints(cons))
	if err != nil {
		return "", application.AddCAASApplicationArg{}, nil, errors.Errorf("making CAAS unit args: %w", err)
	}
	return appName, application.AddCAASApplicationArg{
		BaseAddApplicationArg: arg,
		Scale:                 len(units),
	}, unitArgs, nil
}

func (s *ProviderService) makeApplicationArg(
	ctx context.Context,
	name string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
) (string, application.BaseAddApplicationArg, error) {
	if err := validateCharmAndApplicationParams(
		name,
		args.ReferenceName,
		charm,
		origin,
	); err != nil {
		return "", application.BaseAddApplicationArg{}, errors.Errorf("invalid application args: %w", err)
	}

	if err := validateDownloadInfoParams(origin.Source, args.DownloadInfo); err != nil {
		return "", application.BaseAddApplicationArg{}, errors.Errorf("invalid application args: %w", err)
	}

	if err := validateCreateApplicationResourceParams(charm, args.ResolvedResources, args.PendingResources); err != nil {
		return "", application.BaseAddApplicationArg{}, errors.Errorf("create application: %w", err)
	}

	if err := validateDeviceConstraints(args.Devices, charm.Meta()); err != nil {
		return "", application.BaseAddApplicationArg{}, errors.Errorf("validating device constraints: %w", err)
	}

	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return "", application.BaseAddApplicationArg{}, errors.Errorf("getting model type: %w", err)
	}
	appArg, err := makeCreateApplicationArgs(ctx, s.st, s.storageRegistryGetter, modelType, charm, origin, args)
	if err != nil {
		return "", application.BaseAddApplicationArg{}, errors.Errorf("creating application args: %w", err)
	}
	// We know that the charm name is valid, so we can use it as the application
	// name if that is not provided.
	if name == "" {
		// Annoyingly this should be the reference name, but that's not
		// true in the previous code. To keep compatibility, we'll use the
		// charm name.
		name = appArg.Charm.Metadata.Name
	}

	// Adding units with storage needs to know the kind of storage supported
	// by the underlying provider so gather that here as it needs to be
	// done outside a transaction.
	registry, err := s.storageRegistryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return "", application.BaseAddApplicationArg{}, err
	}

	if len(appArg.Storage) > 0 {
		appArg.StoragePoolKind = make(map[string]storage.StorageKind)
	}
	for _, arg := range appArg.Storage {
		p, err := s.poolStorageProvider(ctx, registry, arg.PoolNameOrType)
		if err != nil {
			return "", application.BaseAddApplicationArg{}, err
		}
		if p.Supports(storage.StorageKindFilesystem) {
			appArg.StoragePoolKind[arg.PoolNameOrType] = storage.StorageKindFilesystem
		}
		if p.Supports(storage.StorageKindBlock) {
			appArg.StoragePoolKind[arg.PoolNameOrType] = storage.StorageKindBlock
		}
	}
	return name, appArg, nil
}

func (s *ProviderService) precheckInstances(
	ctx context.Context,
	platform deployment.Platform,
	unitArgs []application.AddUnitArg,
) error {
	provider, err := s.provider(ctx)
	if err != nil {
		return errors.Errorf("getting provider: %w", err)
	}

	base, err := encodeApplicationBase(platform)
	if err != nil {
		return errors.Errorf("encoding application base: %w", err)
	}

	for _, unitArg := range unitArgs {
		if err := provider.PrecheckInstance(ctx, environs.PrecheckInstanceParams{
			Base:        base,
			Placement:   encodeUnitPlacement(unitArg.Placement),
			Constraints: constraints.EncodeConstraints(unitArg.Constraints),
		}); err != nil {
			return errors.Errorf("pre-checking instances: %w", err)
		}
	}
	return nil
}

func (s *ProviderService) makeApplicationConstraints(ctx context.Context, appUUID coreapplication.ID) (coreconstraints.Value, error) {
	appCons, err := s.st.GetApplicationConstraints(ctx, appUUID)
	if err != nil {
		return coreconstraints.Value{}, errors.Errorf("getting application constraints: %w", err)
	}

	// We must get the charm to know if it's a subordinate.
	charm, err := s.st.GetCharmByApplicationID(ctx, appUUID)
	if err != nil {
		return coreconstraints.Value{}, errors.Errorf("getting application charm for subordinate: %w", err)
	}

	cons, err := s.mergeApplicationAndModelConstraints(ctx, appCons, charm.Metadata.Subordinate)
	if err != nil {
		return coreconstraints.Value{}, errors.Capture(err)
	}

	return cons, nil
}

func (s *ProviderService) constraintsValidator(ctx context.Context) (coreconstraints.Validator, error) {
	provider, err := s.provider(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		// Not validating constraints, as the provider doesn't support it.
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	validator, err := provider.ConstraintsValidator(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return validator, nil
}

func (s *ProviderService) mergeApplicationAndModelConstraints(ctx context.Context, appCons constraints.Constraints, isSubordinate bool) (coreconstraints.Value, error) {
	// If the provider doesn't support constraints validation, then we can
	// just return the zero value.
	validator, err := s.constraintsValidator(ctx)
	if err != nil || validator == nil {
		return coreconstraints.Value{}, errors.Capture(err)
	}

	modelCons, err := s.st.GetModelConstraints(ctx)
	if err != nil && !errors.Is(err, modelerrors.ConstraintsNotFound) {
		return coreconstraints.Value{}, errors.Errorf("retrieving model constraints constraints: %w	", err)
	}

	mergedCons, err := validator.Merge(constraints.EncodeConstraints(appCons), constraints.EncodeConstraints(modelCons))
	if err != nil {
		return coreconstraints.Value{}, errors.Errorf("merging application and model constraints: %w", err)
	}

	// Always ensure that we snapshot the application architecture when adding
	// the application. If no architecture in the constraints, then look at
	// the model constraints. If no architecture is found in the model, use the
	// default architecture (amd64).
	snapshotCons := mergedCons
	if !isSubordinate && !snapshotCons.HasArch() {
		a := coreconstraints.ArchOrDefault(snapshotCons, nil)
		snapshotCons.Arch = &a
	}

	return snapshotCons, nil
}

func (s *ProviderService) validateConstraints(ctx context.Context, cons coreconstraints.Value) error {
	validator, err := s.constraintsValidator(ctx)
	if err != nil {
		return errors.Capture(err)
	} else if validator == nil {
		return nil
	}

	unsupported, err := validator.Validate(cons)
	if len(unsupported) > 0 {
		s.logger.Warningf(ctx,
			"unsupported constraints: %v", strings.Join(unsupported, ","))
	} else if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (s *ProviderService) poolStorageProvider(
	ctx context.Context,
	registry storage.ProviderRegistry,
	poolNameOrType string,
) (storage.Provider, error) {
	poolUUID, err := s.st.GetStoragePoolUUID(ctx, poolNameOrType)
	if errors.Is(err, storageerrors.PoolNotFoundError) {
		// If there's no pool called poolNameOrType, maybe a provider type
		// has been specified directly.
		providerType := storage.ProviderType(poolNameOrType)
		aProvider, registryErr := registry.StorageProvider(providerType)
		if registryErr != nil {
			// The name can't be resolved as a storage provider type,
			// so return the original "pool not found" error.
			return nil, errors.Capture(err)
		}
		return aProvider, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}
	pool, err := s.st.GetStoragePool(ctx, poolUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	providerType := storage.ProviderType(pool.Provider)
	aProvider, err := registry.StorageProvider(providerType)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return aProvider, nil
}

func makeCreateApplicationArgs(
	ctx context.Context,
	state State,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	modelType coremodel.ModelType,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
) (application.BaseAddApplicationArg, error) {
	storageDirectives := make(map[string]storage.Directive)
	for n, sc := range args.Storage {
		storageDirectives[n] = sc
	}

	meta := charm.Meta()

	var err error
	if storageDirectives, err = addDefaultStorageDirectives(ctx, state, modelType, storageDirectives, meta.Storage); err != nil {
		return application.BaseAddApplicationArg{}, errors.Errorf("adding default storage directives: %w", err)
	}
	if err := validateStorageDirectives(ctx, state, storageRegistryGetter, modelType, storageDirectives, meta); err != nil {
		return application.BaseAddApplicationArg{}, errors.Errorf("invalid storage directives: %w", err)
	}

	// When encoding the charm, this will also validate the charm metadata,
	// when parsing it.
	ch, _, err := encodeCharm(charm)
	if err != nil {
		return application.BaseAddApplicationArg{}, errors.Errorf("encoding charm: %w", err)
	}

	revision := -1
	if origin.Revision != nil {
		revision = *origin.Revision
	}

	source, err := encodeCharmSource(origin.Source)
	if err != nil {
		return application.BaseAddApplicationArg{}, errors.Errorf("encoding charm source: %w", err)
	}

	ch.Source = source
	ch.ReferenceName = args.ReferenceName
	ch.Revision = revision
	ch.Hash = origin.Hash
	ch.ArchivePath = args.CharmStoragePath
	ch.ObjectStoreUUID = args.CharmObjectStoreUUID
	ch.Architecture = encodeArchitecture(origin.Platform.Architecture)

	// If we have a storage path, then we know the charm is available.
	// This is passive for now, but once we update the application, the presence
	// of the object store UUID will be used to determine if the charm is
	// available.
	ch.Available = args.CharmStoragePath != ""

	channelArg, platformArg, err := encodeChannelAndPlatform(origin)
	if err != nil {
		return application.BaseAddApplicationArg{}, errors.Errorf("encoding charm origin: %w", err)
	}

	applicationConfig, err := encodeApplicationConfig(args.ApplicationConfig, ch.Config)
	if err != nil {
		return application.BaseAddApplicationArg{}, errors.Errorf("encoding application config: %w", err)
	}

	applicationStatus, err := encodeWorkloadStatus(args.ApplicationStatus)
	if err != nil {
		return application.BaseAddApplicationArg{}, errors.Errorf("encoding application status: %w", err)
	}

	return application.BaseAddApplicationArg{
		Charm:             ch,
		CharmDownloadInfo: args.DownloadInfo,
		Platform:          platformArg,
		Channel:           channelArg,
		EndpointBindings:  args.EndpointBindings,
		Resources:         makeResourcesArgs(args.ResolvedResources),
		PendingResources:  args.PendingResources,
		Storage:           makeStorageArgs(storageDirectives),
		Config:            applicationConfig,
		Settings:          args.ApplicationSettings,
		Status:            applicationStatus,
		Devices:           args.Devices,
		IsController:      args.IsController,
	}, nil
}

func encodeApplicationBase(platform deployment.Platform) (corebase.Base, error) {
	var osName string
	switch platform.OSType {
	case deployment.Ubuntu:
		osName = "ubuntu"
	default:
		return corebase.Base{}, errors.Errorf("unsupported OS type %q", platform.OSType)
	}

	return corebase.Base{
		OS:      osName,
		Channel: corebase.Channel{Track: platform.Channel},
	}, nil
}

func encodeUnitPlacement(placement deployment.Placement) string {
	// We only support provider placements, so if the placement type is not
	// a provider, we return an empty string.
	if placement.Type != deployment.PlacementTypeProvider {
		return ""
	}

	return placement.Directive
}
