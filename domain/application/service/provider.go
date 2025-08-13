// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/assumes"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	coreconstraints "github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/environs"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/password"
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

	agentVersionGetter      AgentVersionGetter
	provider                providertracker.ProviderGetter[Provider]
	caasApplicationProvider providertracker.ProviderGetter[CAASProvider]

	storageProviderValidator StorageProviderValidator
}

// NewProviderService returns a new Service for interacting with a models state.
func NewProviderService(
	st State,
	leaderEnsurer leadership.Ensurer,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
	caasApplicationProvider providertracker.ProviderGetter[CAASProvider],
	storageProviderValidator StorageProviderValidator,
	charmStore CharmStore,
	statusHistory StatusHistory,
	clock clock.Clock,
	logger logger.Logger,
) *ProviderService {
	return &ProviderService{
		Service: NewService(
			st,
			leaderEnsurer,
			charmStore,
			statusHistory,
			clock,
			logger,
		),
		agentVersionGetter:       agentVersionGetter,
		provider:                 provider,
		caasApplicationProvider:  caasApplicationProvider,
		storageProviderValidator: storageProviderValidator,
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
	if err := s.precheckInstances(ctx, appArg.Platform,
		transform.Slice(unitArgs, func(arg application.AddIAASUnitArg) application.AddUnitArg {
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
		if err := s.statusHistory.RecordStatus(
			ctx, status.ApplicationNamespace.WithID(appName), *args.ApplicationStatus,
		); err != nil {
			s.logger.Warningf(ctx, "recording IAAS application status history: %w", err)
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
	preCheckArgs := transform.Slice(unitArgs, func(arg application.AddCAASUnitArg) application.AddUnitArg {
		return arg.AddUnitArg
	})
	if err := s.precheckInstances(ctx, appArg.Platform, preCheckArgs); err != nil {
		return "", errors.Errorf("prechecking instances: %w", err)
	}

	appID, err := s.st.CreateCAASApplication(ctx, appName, appArg, unitArgs)
	if err != nil {
		return "", errors.Errorf("creating CAAS application %q: %w", appName, err)
	}

	s.logger.Infof(ctx, "created CAAS application %q with ID %q", appName, appID)

	if args.ApplicationStatus != nil {
		if err := s.statusHistory.RecordStatus(
			ctx, status.ApplicationNamespace.WithID(appName), *args.ApplicationStatus,
		); err != nil {
			s.logger.Warningf(ctx, "recording CAAS application status history: %w", err)
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
func (s *ProviderService) SetApplicationConstraints(
	ctx context.Context, appID coreapplication.ID, cons coreconstraints.Value,
) error {
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

// AddIAASUnits adds the specified units to the IAAS application, returning an
// error satisfying [applicationerrors.ApplicationNotFoundError] if the
// application doesn't exist. If no units are provided, it will return nil.
func (s *ProviderService) AddIAASUnits(
	ctx context.Context, appName string, units ...AddIAASUnitArg,
) ([]coreunit.Name, []coremachine.Name, error) {
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

	storageDirectives, err := s.st.GetApplicationStorageDirectives(ctx, appUUID)
	if err != nil {
		return nil, nil, errors.Errorf(
			"getting application %q storage directives: %w",
			appName, err,
		)
	}

	args, err := s.makeIAASUnitArgs(
		units, storageDirectives, origin.Platform, constraints.DecodeConstraints(cons),
	)
	if err != nil {
		return nil, nil, errors.Errorf("making IAAS unit args: %w", err)
	}

	if err := s.precheckInstances(
		ctx, origin.Platform, transform.Slice(args, func(arg application.AddIAASUnitArg) application.AddUnitArg {
			return arg.AddUnitArg
		}),
	); err != nil {
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
func (s *ProviderService) AddCAASUnits(
	ctx context.Context, appName string, units ...AddUnitArg,
) ([]coreunit.Name, error) {
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

	storageDirectives, err := s.st.GetApplicationStorageDirectives(ctx, appUUID)
	if err != nil {
		return nil, errors.Errorf(
			"getting application %q storage directives: %w",
			appName, err,
		)
	}

	args, err := s.makeCAASUnitArgs(
		units, storageDirectives, constraints.DecodeConstraints(cons),
	)
	if err != nil {
		return nil, errors.Errorf("making CAAS unit args: %w", err)
	}

	origin, err := s.st.GetApplicationCharmOrigin(ctx, appUUID)
	if err != nil {
		return nil, errors.Errorf("getting application platform: %w", err)
	}
	preCheckArgs := transform.Slice(args, func(arg application.AddCAASUnitArg) application.AddUnitArg {
		return arg.AddUnitArg
	})
	if err := s.precheckInstances(ctx, origin.Platform, preCheckArgs); err != nil {
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
//
// The following errors may occur:
// - [applicationerrors.ApplicationNotFoundError] if the application doesn't
// exist. If the unit life is Dead, an error satisfying
func (s *ProviderService) RegisterCAASUnit(
	ctx context.Context,
	params application.RegisterCAASUnitParams,
) (coreunit.Name, string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if params.ProviderID == "" {
		return "", "", errors.Errorf("provider id %w", coreerrors.NotValid)
	}

	appUUID, err := s.st.GetApplicationIDByName(ctx, params.ApplicationName)
	if err != nil {
		return "", "", errors.Capture(err)
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
	var caasUnit *caas.Unit
	for _, v := range pods {
		p := v
		if p.Id == params.ProviderID {
			caasUnit = &p
			break
		}
	}
	if caasUnit == nil {
		return "", "", errors.Errorf("pod %s in provider %w", params.ProviderID, coreerrors.NotFound)
	}

	if caasUnit.Address != "" {
		registerArgs.Address = &caasUnit.Address
	}
	if len(caasUnit.Ports) != 0 {
		registerArgs.Ports = &caasUnit.Ports
	}

	storageArg, err := s.getRegisterCAASUnitStorageArgs(
		ctx, appUUID, unitName, caasUnit.FilesystemInfo,
	)
	if err != nil {
		return "", "", errors.Errorf(
			"creating storage registrations for caas unit %q: %w",
			unitName, err,
		)
	}

	registerArgs.RegisterUnitStorageArg = storageArg

	err = s.st.RegisterCAASUnit(ctx, appName, registerArgs)
	if err != nil {
		return "", "", errors.Errorf(
			"saving caas unit %q: %w", registerArgs.UnitName, err,
		)
	}
	return unitName, pass, nil
}

// ResolveApplicationConstraints resolves given application constraints, taking
// into account the model constraints, and any hard-coded default constraint values
// that exist.
func (s *ProviderService) ResolveApplicationConstraints(
	ctx context.Context, appCons coreconstraints.Value,
) (coreconstraints.Value, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.resolveApplicationConstraints(ctx, appCons)
}

// getRegisterCAASUnitStorageArgs is responsible for fetching the storage
// arguments required to register a CAAS unit in the model. This func considers
// pre existing storage already in the model for the unit and that of storage
// which exists matching the provider's unique ids.
//
// This function will first use all the existing storage in the model for the
// unit before creating new storage to meet the storage directives of the unit.
// Storage created by this func will be associated with the providers
// information on first creation. All storage created and re-used will also now
// be owned by the unit being registered.
//
// The following errors may be expected:
// - [applicationerrors.ApplicationNotFound] when the application no longer
// exists.
func (s *ProviderService) getRegisterCAASUnitStorageArgs(
	ctx context.Context,
	appUUID coreapplication.ID,
	unitName coreunit.Name,
	providerFilesystemInfo []caas.FilesystemInfo,
) (application.RegisterUnitStorageArg, error) {
	// We don't consider or look at volume information for the caas filesystem.
	// Volume sequences have previously been used as a means of associating
	// provider information to storage instances. It is impossible to keep these
	// two sources in sync so instead we rely on the unique id from the
	// provider.
	//
	// For CAAS this provider value lives in caas.FilesystemInfo.FilesystemId.
	//
	// Collect the set of provider ids and form a mapping from charm storage
	// name.
	providerIDs := make([]string, 0, len(providerFilesystemInfo))
	providerIDName := make(map[string]domainstorage.Name, len(providerFilesystemInfo))
	for _, fsInfo := range providerFilesystemInfo {
		providerIDs = append(providerIDs, fsInfo.FilesystemId)
		providerIDName[fsInfo.FilesystemId] = domainstorage.Name(fsInfo.StorageName)
	}

	// We fetch all existing storage instances in the model that are using one
	// of the provider ids.
	existingProviderStorage, err := s.st.GetStorageInstancesForProviderIDs(
		ctx, appUUID, providerIDs,
	)
	if err != nil {
		return application.RegisterUnitStorageArg{}, errors.Errorf(
			"getting existing storage instances based on observed provider ids: %w",
			err,
		)
	}

	var (
		existingUnitStorage map[domainstorage.Name][]domainstorage.StorageInstanceUUID
		directivesToFollow  []application.StorageDirective
	)

	// Check if the unit already exists.
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil && !errors.Is(err, applicationerrors.UnitNotFound) {
		return application.RegisterUnitStorageArg{}, errors.Errorf(
			"checking if unit %q already exists: %w", unitName, err,
		)
	}

	if unitUUID != "" {
		// If the unit exists, we will get the already established storage
		// directives including and storage that the unit currently owns.
		directivesToFollow, existingUnitStorage, err = s.getUnitStorageInfo(
			ctx, unitUUID,
		)
		if err != nil {
			return application.RegisterUnitStorageArg{}, errors.Errorf(
				"getting existing unit %q storage information for registration: %w",
				unitUUID, err,
			)
		}
	} else {
		// If the unit does not exist, we will instead get and follow the
		// storage directives of the application.
		directivesToFollow, err = s.st.GetApplicationStorageDirectives(
			ctx, appUUID,
		)
		if err != nil {
			return application.RegisterUnitStorageArg{}, errors.Errorf(
				"getting application %q storage directives: %w", appUUID, err,
			)
		}
		existingUnitStorage = map[domainstorage.Name][]domainstorage.StorageInstanceUUID{}
	}

	// We need to walk through all of the storage in the model that is using one
	// of the provider ids and make it available under the units existing
	// storage.
	//
	// It is possible for storage to exist in the model that is not currently
	// owned by a unit. This happens in CAAS models where a unit going away does
	// not remove provisioned storage.
	for providerId, instanceUUID := range existingProviderStorage {
		storageName := providerIDName[providerId]

		// If the storage has already been identified as belonging to the unit
		// there is nothing more to do.
		if slices.Contains(existingUnitStorage[storageName], instanceUUID) {
			continue
		}

		existingUnitStorage[storageName] = append(
			existingUnitStorage[storageName], instanceUUID,
		)
	}

	unitStorageArgs, err := makeUnitStorageArgs(
		directivesToFollow, existingUnitStorage,
	)
	if err != nil {
		return application.RegisterUnitStorageArg{}, errors.Capture(err)
	}

	// Build a list of provider ids that are not associated with a storage
	// instance as of yet. This would be the case for a new unit getting
	// registered for the first time.
	unassignedProviderIDs := slices.DeleteFunc(providerIDs, func(id string) bool {
		_, exists := existingProviderStorage[id]
		return exists
	})

	// Now map all of the unassigned provider ids to the storage name that
	// they can be assigned to.
	unassignedNameMapping := map[domainstorage.Name][]string{}
	for _, unassignedProviderID := range unassignedProviderIDs {
		nameIDs := unassignedNameMapping[providerIDName[unassignedProviderID]]
		nameIDs = append(nameIDs, unassignedProviderID)
		unassignedNameMapping[providerIDName[unassignedProviderID]] = nameIDs
	}

	// filesystemProviderIDs is a mapping of new filesystem uuids that are to be
	// created and the provider id for the filesystem.
	filesystemProviderIDs := map[domainstorageprov.FilesystemUUID]string{}

	// We need to walk through all of the new storage instances being created
	// and assign a provider id to them.
	for _, inst := range unitStorageArgs.StorageInstances {
		// We can safely assume that all CAAS units create filesystems but we
		// are not in charge of the business logic here. So instead let us just
		// skip this instance if it does not have a non filesystem uuid.
		if inst.FilesystemUUID == nil {
			continue
		}

		nameIDs := unassignedNameMapping[inst.Name]
		if len(nameIDs) == 0 {
			// This should never happen, but it does mean we have a disjoint
			// situation between what the provider is creating and the
			// information in the database.
			return application.RegisterUnitStorageArg{}, errors.Errorf(
				"no provider id exists to assign to unit %q new storage instance %q",
				unitName, inst.Name,
			)
		}

		// Pop the first unassigned provider id and give it to the new
		// filesystem.
		filesystemProviderIDs[*inst.FilesystemUUID] = nameIDs[0]
		unassignedNameMapping[inst.Name] = nameIDs[1:]
	}

	return application.RegisterUnitStorageArg{
		CreateUnitStorageArg:  unitStorageArgs,
		FilesystemProviderIDs: filesystemProviderIDs,
	}, nil
}

func (s *ProviderService) makeIAASApplicationArg(ctx context.Context,
	name string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
	units ...AddIAASUnitArg,
) (string, application.AddIAASApplicationArg, []application.AddIAASUnitArg, error) {
	if err := s.validateCreateApplicationArgs(ctx, name, charm, origin, args); err != nil {
		return "", application.AddIAASApplicationArg{}, nil,
			errors.Errorf("validating create application args: %w", err)
	}

	// Subordinate applications are not allowed to have constraints, so no point
	// in trying to resolve them.
	// Also, we know that the charm must have a non-nil meta, since we have already
	// validated the args.
	cons := coreconstraints.Value{}
	if !charm.Meta().Subordinate {
		var err error
		cons, err = s.resolveApplicationConstraints(ctx, args.Constraints)
		if err != nil {
			return "", application.AddIAASApplicationArg{}, nil,
				errors.Errorf("merging IAAS application and model constraints: %w", err)
		}

		// Sometimes the arch on the origin platform is not set. But sometimes an arch
		// is passed in through constraints instead (or at least, after resolve we
		// will have a value). Ensure we these two params don't contradict each other,
		// and ensure they are set to the same value.
		if origin.Platform.Architecture != "" && origin.Platform.Architecture != *cons.Arch {
			return "", application.AddIAASApplicationArg{}, nil,
				errors.Errorf("arch in platform and constraints for application do not match")
		}
		origin.Platform.Architecture = *cons.Arch
	}

	appName, arg, err := s.makeApplicationArg(ctx, name, charm, origin, args)
	if err != nil {
		return "", application.AddIAASApplicationArg{}, nil, errors.Errorf("preparing IAAS application args: %w", err)
	}

	storageDirectives := makeStorageDirectiveFromApplicationArg(
		arg.StorageDirectives,
	)
	unitArgs, err := s.makeIAASUnitArgs(
		units, storageDirectives, arg.Platform, constraints.DecodeConstraints(cons),
	)
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
) (string, application.AddCAASApplicationArg, []application.AddCAASUnitArg, error) {
	if err := s.validateCreateApplicationArgs(ctx, name, charm, origin, args); err != nil {
		return "", application.AddCAASApplicationArg{}, nil,
			errors.Errorf("validating create application args: %w", err)
	}

	cons, err := s.resolveApplicationConstraints(ctx, args.Constraints)
	if err != nil {
		return "", application.AddCAASApplicationArg{}, nil,
			errors.Errorf("merging CAAS application and model constraints: %w", err)
	}

	// Sometimes the arch on the origin platform is not set. But sometimes an arch
	// is passed in through constraints instead (or at least, after resolve we
	// will have a value). Ensure we these two params don't contradict each other,
	// and ensure they are set to the same value.
	if origin.Platform.Architecture != "" && origin.Platform.Architecture != *cons.Arch {
		return "", application.AddCAASApplicationArg{}, nil,
			errors.Errorf("arch in platform and constraints for application do not match")
	}
	origin.Platform.Architecture = *cons.Arch

	appName, arg, err := s.makeApplicationArg(ctx, name, charm, origin, args)
	if err != nil {
		return "", application.AddCAASApplicationArg{}, nil, errors.Errorf("preparing CAAS application args: %w", err)
	}

	storageDirectives := makeStorageDirectiveFromApplicationArg(arg.StorageDirectives)
	unitArgs, err := s.makeCAASUnitArgs(
		units, storageDirectives, constraints.DecodeConstraints(cons),
	)
	if err != nil {
		return "", application.AddCAASApplicationArg{}, nil, errors.Errorf("making CAAS unit args: %w", err)
	}
	return appName, application.AddCAASApplicationArg{
		BaseAddApplicationArg: arg,
		Scale:                 len(units),
	}, unitArgs, nil
}

func (s *ProviderService) validateCreateApplicationArgs(
	ctx context.Context,
	name string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
) error {
	if err := validateCharmAndApplicationParams(
		name,
		args.ReferenceName,
		charm,
		origin,
	); err != nil {
		return errors.Errorf("invalid application args: %w", err)
	}

	err := validateApplicationStorageDirectiveParams(
		ctx,
		charm.Meta().Storage,
		args.StorageDirectiveOverrides,
		s.storageProviderValidator,
	)
	if err != nil {
		return errors.Errorf(
			"invalid storage directive overrides: %w", err,
		)
	}

	if err := validateDownloadInfoParams(origin.Source, args.DownloadInfo); err != nil {
		return errors.Errorf("invalid application args: %w", err)
	}

	if err := validateCreateApplicationResourceParams(charm, args.ResolvedResources, args.PendingResources); err != nil {
		return errors.Errorf("create application: %w", err)
	}

	if err := validateDeviceConstraints(args.Devices, charm.Meta()); err != nil {
		return errors.Errorf("validating device constraints: %w", err)
	}

	return nil
}

func (s *ProviderService) makeApplicationArg(
	ctx context.Context,
	name string,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
) (string, application.BaseAddApplicationArg, error) {
	appArg, err := makeCreateApplicationArgs(ctx, s.st, charm, origin, args)
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

func (s *ProviderService) makeApplicationConstraints(
	ctx context.Context, appUUID coreapplication.ID,
) (coreconstraints.Value, error) {
	appCons, err := s.st.GetApplicationConstraints(ctx, appUUID)
	if err != nil {
		return coreconstraints.Value{}, errors.Errorf("getting application constraints: %w", err)
	}

	cons, err := s.resolveApplicationConstraints(ctx, constraints.EncodeConstraints(appCons))
	if err != nil {
		return coreconstraints.Value{}, errors.Capture(err)
	}

	return cons, nil
}

// constraintsValidator queries the provider for a constraints validator.
// If the provider doesn't support constraints validation, then we return
// a default validator
func (s *ProviderService) constraintsValidator(ctx context.Context) (coreconstraints.Validator, error) {
	provider, err := s.provider(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	validator, err := provider.ConstraintsValidator(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	} else if validator == nil {
		return coreconstraints.NewValidator(), nil
	}

	return validator, nil
}

func (s *ProviderService) resolveApplicationConstraints(
	ctx context.Context, appCons coreconstraints.Value,
) (coreconstraints.Value, error) {
	validator, err := s.constraintsValidator(ctx)
	if err != nil {
		return coreconstraints.Value{}, errors.Capture(err)
	}
	modelCons, err := s.st.GetModelConstraints(ctx)
	if err != nil && !errors.Is(err, modelerrors.ConstraintsNotFound) {
		return coreconstraints.Value{}, errors.Errorf("retrieving model constraints constraints: %w	", err)
	}

	mergedCons, err := validator.Merge(constraints.EncodeConstraints(modelCons), appCons)
	if err != nil {
		return coreconstraints.Value{}, errors.Errorf("merging application and model constraints: %w", err)
	}

	if !mergedCons.HasArch() {
		mergedCons.Arch = ptr(arch.DefaultArchitecture)
	}

	return mergedCons, nil
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

func makeCreateApplicationArgs(
	ctx context.Context,
	storageSt StorageState,
	charm internalcharm.Charm,
	origin corecharm.Origin,
	args AddApplicationArgs,
) (application.BaseAddApplicationArg, error) {
	defaultStorageProviders, err := storageSt.GetDefaultStorageProvisioners(ctx)
	if err != nil {
		return application.BaseAddApplicationArg{}, errors.Errorf(
			"getting default storage provisioners for model: %w", err,
		)
	}

	charmMeta := charm.Meta()
	storageDirectiveArgs := makeApplicationStorageDirectiveArgs(
		args.StorageDirectiveOverrides,
		charmMeta.Storage,
		defaultStorageProviders,
	)

	err = validateApplicationStorageDirectives(charmMeta.Storage, storageDirectiveArgs)
	if err != nil {
		return application.BaseAddApplicationArg{}, errors.Errorf(
			"invalid application storage directives: %w", err,
		)
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
		StorageDirectives: storageDirectiveArgs,
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
