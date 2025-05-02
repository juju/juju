// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strconv"
	"strings"

	"github.com/juju/clock"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/assumes"
	corecharm "github.com/juju/juju/core/charm"
	coreconstraints "github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/status"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/storage"
)

// ProviderService defines a service for interacting with the underlying
// model state.
type ProviderService struct {
	*Service

	modelID            coremodel.UUID
	agentVersionGetter AgentVersionGetter
	provider           providertracker.ProviderGetter[Provider]
	// This provider is separated from [provider] because the
	// [SupportedFeatureProvider] interface is only satisfied by the
	// k8s provider.
	supportedFeatureProvider providertracker.ProviderGetter[SupportedFeatureProvider]
	caasApplicationProvider  providertracker.ProviderGetter[CAASApplicationProvider]
}

// NewProviderService returns a new Service for interacting with a models state.
func NewProviderService(
	st State,
	leaderEnsurer leadership.Ensurer,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	modelID coremodel.UUID,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
	supportedFeatureProvider providertracker.ProviderGetter[SupportedFeatureProvider],
	caasApplicationProvider providertracker.ProviderGetter[CAASApplicationProvider],
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
		modelID:                  modelID,
		agentVersionGetter:       agentVersionGetter,
		provider:                 provider,
		supportedFeatureProvider: supportedFeatureProvider,
		caasApplicationProvider:  caasApplicationProvider,
	}
}

func (s *Service) poolStorageProvider(
	ctx context.Context,
	registry storage.ProviderRegistry,
	poolNameOrType string,
) (storage.Provider, error) {
	pool, err := s.st.GetStoragePoolByName(ctx, poolNameOrType)
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
	providerType := storage.ProviderType(pool.Provider)
	aProvider, err := registry.StorageProvider(providerType)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return aProvider, nil
}

// CreateApplication creates the specified application and units if required,
// returning an error satisfying [applicationerrors.ApplicationAlreadyExists]
// if the application already exists.
func (s *ProviderService) CreateApplication(
	ctx context.Context,
	name string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
	units ...AddUnitArg,
) (coreapplication.ID, error) {
	if err := validateCharmAndApplicationParams(
		name,
		args.ReferenceName,
		charm,
		origin,
	); err != nil {
		return "", errors.Errorf("invalid application args: %w", err)
	}

	if err := validateDownloadInfoParams(origin.Source, args.DownloadInfo); err != nil {
		return "", errors.Errorf("invalid application args: %w", err)
	}

	if err := validateCreateApplicationResourceParams(charm, args.ResolvedResources, args.PendingResources); err != nil {
		return "", errors.Errorf("create application: %w", err)
	}

	if err := validateDeviceConstraints(args.Devices, charm.Meta()); err != nil {
		return "", errors.Errorf("validating device constraints: %w", err)
	}

	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return "", errors.Errorf("getting model type: %w", err)
	}
	appArg, err := makeCreateApplicationArgs(ctx, s.st, s.storageRegistryGetter, modelType, charm, origin, args)
	if err != nil {
		return "", errors.Errorf("creating application args: %w", err)
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

	cons, err := s.mergeApplicationAndModelConstraints(ctx, constraints.DecodeConstraints(args.Constraints))
	if err != nil {
		return "", errors.Errorf("merging application and model constraints: %w", err)
	}

	// Adding units with storage needs to know the kind of storage supported
	// by the underlying provider so gather that here as it needs to be
	// done outside a transaction.
	registry, err := s.storageRegistryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return "", err
	}

	unitArgs, err := s.makeUnitArgs(modelType, units, constraints.DecodeConstraints(cons))
	if err != nil {
		return "", errors.Errorf("making unit args: %w", err)
	}

	if len(appArg.Storage) > 0 {
		appArg.StoragePoolKind = make(map[string]storage.StorageKind)
	}
	for _, arg := range appArg.Storage {
		p, err := s.poolStorageProvider(ctx, registry, arg.PoolNameOrType)
		if err != nil {
			return "", err
		}
		if p.Supports(storage.StorageKindFilesystem) {
			appArg.StoragePoolKind[arg.PoolNameOrType] = storage.StorageKindFilesystem
		}
		if p.Supports(storage.StorageKindBlock) {
			appArg.StoragePoolKind[arg.PoolNameOrType] = storage.StorageKindBlock
		}
	}
	appID, err := s.st.CreateApplication(ctx, name, appArg, unitArgs)
	if err != nil {
		return "", errors.Errorf("creating application %q: %w", name, err)
	}

	s.logger.Infof(ctx, "created application %q with ID %q", name, appID)

	if args.ApplicationStatus != nil {
		if err := s.statusHistory.RecordStatus(ctx, status.ApplicationNamespace.WithID(name), *args.ApplicationStatus); err != nil {
			s.logger.Infof(ctx, "failed recording application status history: %w", err)
		}
	}

	return appID, nil
}

// GetSupportedFeatures returns the set of features that the model makes
// available for charms to use.
// If the agent version cannot be found, an error satisfying
// [modelerrors.NotFound] will be returned.
func (s *ProviderService) GetSupportedFeatures(ctx context.Context) (assumes.FeatureSet, error) {
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

	supportedFeatureProvider, err := s.supportedFeatureProvider(ctx)
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
	if err := appID.Validate(); err != nil {
		return errors.Errorf("application ID: %w", err)
	}
	if err := s.validateConstraints(ctx, cons); err != nil {
		return err
	}

	return s.st.SetApplicationConstraints(ctx, appID, constraints.DecodeConstraints(cons))
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

// AddUnits adds the specified units to the application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application
// doesn't exist.
// If no units are provided, it will return nil.
func (s *ProviderService) AddUnits(ctx context.Context, storageParentDir, appName string, units ...AddUnitArg) error {
	if len(units) == 0 {
		return nil
	}

	if !isValidApplicationName(appName) {
		return applicationerrors.ApplicationNameNotValid
	}

	appUUID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return errors.Errorf("getting application %q id: %w", appName, err)
	}

	charmUUID, err := s.st.GetCharmIDByApplicationName(ctx, appName)
	if err != nil {
		return errors.Errorf("getting charm %q id: %w", appName, err)
	}

	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return errors.Errorf("getting model type: %w", err)
	}

	appCons, err := s.st.GetApplicationConstraints(ctx, appUUID)
	if err != nil {
		return errors.Errorf("getting application %q constraints: %w", appName, err)
	}

	cons, err := s.mergeApplicationAndModelConstraints(ctx, appCons)
	if err != nil {
		return errors.Capture(err)
	}

	args, err := s.makeUnitArgs(modelType, units, constraints.DecodeConstraints(cons))
	if err != nil {
		return errors.Errorf("making unit args: %w", err)
	}

	if modelType == coremodel.IAAS {
		err = s.st.AddIAASUnits(ctx, storageParentDir, appUUID, charmUUID, args...)
	} else {
		err = s.st.AddCAASUnits(ctx, storageParentDir, appUUID, charmUUID, args...)
	}
	if err != nil {
		return errors.Errorf("adding units to application %q: %w", appName, err)
	}

	for _, arg := range args {
		if err := s.recordStatusHistory(ctx, arg.UnitName, arg.UnitStatusArg); err != nil {
			return errors.Errorf("recording status history: %w", err)
		}
	}

	return nil
}

func (s *Service) recordStatusHistory(
	ctx context.Context,
	unitName coreunit.Name,
	statusArg application.UnitStatusArg,
) error {
	// The agent and workload status are required to be provided when adding
	// a unit.
	if statusArg.AgentStatus == nil || statusArg.WorkloadStatus == nil {
		return errors.Errorf("unit %q status not provided", unitName)
	}

	// Force the presence to be recorded as true, as the unit has just been
	// added.
	if agentStatus, err := decodeUnitAgentStatus(&status.UnitStatusInfo[status.UnitAgentStatusType]{
		StatusInfo: *statusArg.AgentStatus,
		Present:    true,
	}); err == nil && agentStatus != nil {
		if err := s.statusHistory.RecordStatus(ctx, status.UnitAgentNamespace.WithID(unitName.String()), *agentStatus); err != nil {
			s.logger.Infof(ctx, "failed recording agent status for unit %q: %v", unitName, err)
		}
	}

	if workloadStatus, err := decodeUnitWorkloadStatus(&status.UnitStatusInfo[status.WorkloadStatusType]{
		StatusInfo: *statusArg.WorkloadStatus,
		Present:    true,
	}); err == nil && workloadStatus != nil {
		if err := s.statusHistory.RecordStatus(ctx, status.UnitWorkloadNamespace.WithID(unitName.String()), *workloadStatus); err != nil {
			s.logger.Infof(ctx, "failed recording workload status for unit %q: %v", unitName, err)
		}
	}

	return nil
}

// CAASUnitTerminating should be called by the CAASUnitTerminationWorker when
// the agent receives a signal to exit. UnitTerminating will return how the
// agent should shutdown.
//
// We pass in a CAAS broker to get app details from the k8s cluster - we will
// probably make it a service attribute once more use cases emerge.
func (s *ProviderService) CAASUnitTerminating(ctx context.Context, unitNameStr string) (bool, error) {
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
	if params.ProviderID == "" {
		return "", "", errors.Errorf("provider id %w", coreerrors.NotValid)
	}

	pass, err := password.RandomPassword()
	if err != nil {
		return "", "", errors.Errorf("generating unit password: %w", err)
	}
	registerArgs := application.RegisterCAASUnitArg{
		ProviderID:       params.ProviderID,
		StorageParentDir: application.StorageParentDir,
		PasswordHash:     password.AgentPasswordHash(pass),
	}

	// We don't support anything other that statefulsets.
	// So the pod name contains the unit number.
	appName := params.ApplicationName
	splitPodName := strings.Split(params.ProviderID, "-")
	ord, err := strconv.Atoi(splitPodName[len(splitPodName)-1])
	if err != nil {
		return "", "", errors.Capture(err)
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

func (s *ProviderService) mergeApplicationAndModelConstraints(ctx context.Context, appCons constraints.Constraints) (coreconstraints.Value, error) {
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

	res, err := validator.Merge(constraints.EncodeConstraints(appCons), constraints.EncodeConstraints(modelCons))
	if err != nil {
		return coreconstraints.Value{}, errors.Errorf("merging application and model constraints: %w", err)
	}

	return res, nil
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
