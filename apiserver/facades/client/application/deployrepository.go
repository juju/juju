// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"
	"strconv"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/kr/pretty"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/unit"
	jujuversion "github.com/juju/juju/core/version"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/environs/bootstrap"
	environsconfig "github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/repository"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// DeployFromRepositoryValidator defines an deploy config validator.
type DeployFromRepositoryValidator interface {
	ValidateArg(context.Context, params.DeployFromRepositoryArg) (deployTemplate, []error)
}

// DeployFromRepository defines an interface for deploying a charm
// from a repository.
type DeployFromRepository interface {
	DeployFromRepository(context.Context, params.DeployFromRepositoryArg) (params.DeployFromRepositoryInfo, []*params.PendingResourceUpload, []error)
}

// DeployFromRepositoryState defines a common set of functions for retrieving state
// objects.
type DeployFromRepositoryState interface {
	AddApplication(state.AddApplicationArgs, objectstore.ObjectStore) (Application, error)
	Machine(string) (Machine, error)

	ReadSequence(name string) (int, error)
}

// DeployFromRepositoryAPI provides the deploy from repository
// API facade for any given version. It is expected that any API
// parameter changes should be performed before entering the API.
type DeployFromRepositoryAPI struct {
	state              DeployFromRepositoryState
	store              objectstore.ObjectStore
	validator          DeployFromRepositoryValidator
	applicationService ApplicationService
	logger             corelogger.Logger
}

// NewDeployFromRepositoryAPI creates a new DeployFromRepositoryAPI.
func NewDeployFromRepositoryAPI(
	state DeployFromRepositoryState, applicationService ApplicationService,
	store objectstore.ObjectStore, validator DeployFromRepositoryValidator,
	logger corelogger.Logger,
) DeployFromRepository {
	return &DeployFromRepositoryAPI{
		state:              state,
		store:              store,
		validator:          validator,
		applicationService: applicationService,
		logger:             logger,
	}
}

func (api *DeployFromRepositoryAPI) DeployFromRepository(ctx context.Context, arg params.DeployFromRepositoryArg) (params.DeployFromRepositoryInfo, []*params.PendingResourceUpload, []error) {
	api.logger.Tracef(context.TODO(), "deployOneFromRepository(%s)", pretty.Sprint(arg))
	// Validate the args.
	dt, errs := api.validator.ValidateArg(ctx, arg)

	if len(errs) > 0 {
		return params.DeployFromRepositoryInfo{}, nil, errs
	}

	info := params.DeployFromRepositoryInfo{
		Architecture: dt.origin.Platform.Architecture,
		Base: params.Base{
			Name:    dt.origin.Platform.OS,
			Channel: dt.origin.Platform.Channel,
		},
		Channel:          dt.origin.Channel.String(),
		EffectiveChannel: nil,
		Name:             dt.applicationName,
		Revision:         dt.charmURL.Revision,
	}
	if dt.dryRun {
		return info, nil, nil
	}
	// Queue async charm download.
	// AddCharmMetadata returns no error if the charm
	// has already been queue'd or downloaded.
	stOrigin, err := StateCharmOrigin(dt.origin)
	if err != nil {
		return params.DeployFromRepositoryInfo{}, nil, []error{errors.Trace(err)}
	}

	// To ensure dqlite unit names match those created in mongo, grab the next unit
	// sequence number before writing the mongo units.
	nextUnitNum, err := api.state.ReadSequence(dt.applicationName)
	if err != nil {
		return params.DeployFromRepositoryInfo{}, nil, []error{errors.Trace(err)}
	}

	_, err = api.state.AddApplication(state.AddApplicationArgs{
		ApplicationConfig: dt.applicationConfig,
		AttachStorage:     dt.attachStorage,
		Charm:             dt.charm,
		CharmURL:          dt.charmURL.String(),
		CharmConfig:       dt.charmSettings,
		CharmOrigin:       stOrigin,
		Constraints:       dt.constraints,
		Devices:           stateDeviceConstraints(arg.Devices),
		EndpointBindings:  dt.endpoints,
		Name:              dt.applicationName,
		NumUnits:          dt.numUnits,
		Placement:         dt.placement,
		Storage:           stateStorageDirectives(dt.storage),
	}, api.store)
	if err != nil {
		return params.DeployFromRepositoryInfo{}, nil, []error{errors.Trace(err)}
	}

	unitArgs := make([]applicationservice.AddUnitArg, dt.numUnits)
	for i := 0; i < dt.numUnits; i++ {
		unitName, err := unit.NewNameFromParts(dt.applicationName, nextUnitNum+i)
		if err != nil {
			return params.DeployFromRepositoryInfo{}, nil, []error{errors.Trace(err)}
		}
		unitArgs[i].UnitName = unitName
	}

	_, err = api.applicationService.CreateApplication(ctx, dt.applicationName, dt.charm, dt.origin,
		applicationservice.AddApplicationArgs{
			ReferenceName: dt.charmURL.Name,
			Storage:       dt.storage,
			// We always have download info for a charm from the charmhub store.
			DownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:         applicationcharm.ProvenanceDownload,
				CharmhubIdentifier: dt.downloadInfo.CharmhubIdentifier,
				DownloadURL:        dt.downloadInfo.DownloadURL,
				DownloadSize:       dt.downloadInfo.DownloadSize,
			},
			ResolvedResources: dt.resolvedResources,
			EndpointBindings:  transformBindings(dt.endpoints),
		}, unitArgs...)
	if err != nil {
		return params.DeployFromRepositoryInfo{}, nil, []error{errors.Trace(err)}
	}

	return info, dt.resourcesToUpload, nil
}

// resolveResources resolves and maps resources for deployment, handling input
// resource revisions or files. It validates existence in the charm repository
// and returns resolved resource details or an error.  Errors are not terminal,
// and will be collected and returned altogether.
func (v *deployFromRepositoryValidator) resolveResources(
	ctx context.Context,
	curl *charm.URL,
	origin corecharm.Origin,
	deployResArg map[string]string,
	resMeta map[string]resource.Meta,
) (applicationservice.ResolvedResources, []*params.PendingResourceUpload, error) {
	var resourcesToUpload []*params.PendingResourceUpload
	var resources []resource.Resource

	// Solve charm meta against resources args.
	for name, meta := range resMeta {
		r := resource.Resource{
			Meta:     meta,
			Origin:   resource.OriginStore,
			Revision: -1,
		}
		if deployValue, ok := deployResArg[name]; ok {
			// Resource flag is used on the cli, either a resource revision, or a filename.
			providedRev, err := strconv.Atoi(deployValue)
			if err != nil {
				// A file is coming from the client.
				r.Origin = resource.OriginUpload

				// Record resources that the client needs to upload.
				resourcesToUpload = append(resourcesToUpload, &params.PendingResourceUpload{
					Name:     meta.Name,
					Type:     meta.Type.String(),
					Filename: deployValue,
				})
			} else {
				// A revision is coming from client.
				r.Revision = providedRev
			}
		}
		resources = append(resources, r)
	}

	// Solve revision against charm repository.
	repo, err := v.getCharmRepository(ctx)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	resolvedResources, resolveErr := repo.ResolveResources(ctx, resources, corecharm.CharmID{URL: curl, Origin: origin})
	if resolveErr != nil {
		return nil, nil, resolveErr
	}

	// Convert it in resolved resources.
	result := make(applicationservice.ResolvedResources, 0, len(resolvedResources))
	for _, res := range resolvedResources {
		var revision *int
		if res.Revision >= 0 {
			revision = &res.Revision
		}
		result = append(result, applicationservice.ResolvedResource{
			Name:     res.Name,
			Origin:   res.Origin,
			Revision: revision,
		})
	}
	return result, resourcesToUpload, nil
}

type deployTemplate struct {
	applicationConfig *config.Config
	applicationName   string
	attachStorage     []names.StorageTag
	charm             charm.Charm
	charmSettings     charm.Settings
	charmURL          *charm.URL
	constraints       constraints.Value
	endpoints         map[string]string
	dryRun            bool
	force             bool
	numUnits          int
	origin            corecharm.Origin
	placement         []*instance.Placement
	resources         map[string]string
	storage           map[string]storage.Directive
	resourcesToUpload []*params.PendingResourceUpload
	resolvedResources applicationservice.ResolvedResources
	downloadInfo      corecharm.DownloadInfo
}

type validatorConfig struct {
	charmhubHTTPClient facade.HTTPClient
	caasBroker         CaasBrokerInterface
	modelInfo          model.ModelInfo
	modelConfigService ModelConfigService
	applicationService ApplicationService
	machineService     MachineService
	registry           storage.ProviderRegistry
	state              DeployFromRepositoryState
	storageService     StorageService
	logger             corelogger.Logger
}

func makeDeployFromRepositoryValidator(ctx context.Context, cfg validatorConfig) DeployFromRepositoryValidator {
	v := &deployFromRepositoryValidator{
		charmhubHTTPClient: cfg.charmhubHTTPClient,
		modelInfo:          cfg.modelInfo,
		modelConfigService: cfg.modelConfigService,
		applicationService: cfg.applicationService,
		machineService:     cfg.machineService,
		state:              cfg.state,
		newCharmHubRepository: func(cfg repository.CharmHubRepositoryConfig) (corecharm.Repository, error) {
			return repository.NewCharmHubRepository(cfg)
		},
		newStateBindings: func(st any, givenMap map[string]string) (Bindings, error) {
			return state.NewBindings(st, givenMap)
		},
		logger: cfg.logger,
	}
	if cfg.modelInfo.Type == model.CAAS {
		return &caasDeployFromRepositoryValidator{
			caasBroker:     cfg.caasBroker,
			registry:       cfg.registry,
			storageService: cfg.storageService,
			validator:      v,
			caasPrecheckFunc: func(dt deployTemplate) error {
				attachStorage := make([]string, len(dt.attachStorage))
				for i, tag := range dt.attachStorage {
					attachStorage[i] = tag.Id()
				}
				cdp := caasDeployParams{
					applicationName: dt.applicationName,
					attachStorage:   attachStorage,
					charm:           dt.charm,
					config:          nil,
					placement:       dt.placement,
					storage:         dt.storage,
				}
				return cdp.precheck(ctx, v.modelConfigService, cfg.storageService, cfg.registry, cfg.caasBroker)
			},
		}
	}
	return &iaasDeployFromRepositoryValidator{
		validator: v,
	}
}

type deployFromRepositoryValidator struct {
	modelInfo          model.ModelInfo
	modelConfigService ModelConfigService
	applicationService ApplicationService
	machineService     MachineService
	state              DeployFromRepositoryState

	// For testing using mocks.
	newCharmHubRepository func(repository.CharmHubRepositoryConfig) (corecharm.Repository, error)
	charmhubHTTPClient    facade.HTTPClient

	// For testing using mocks.
	newStateBindings func(st any, givenMap map[string]string) (Bindings, error)

	logger corelogger.Logger
}

// Validating arguments to deploy a charm.
// General (see deployFromRepositoryValidator)
//   - Resolve the charm and ensure it exists in a repository
//   - Ensure supplied resources exist
//   - Find repository resources to be used.
//   - Check machine placement against current deployment - does not include
//     the caas check below.
//   - Find a charm to match the provided name and architecture at a minimum,
//     and base, revision, and channel if provided.
//   - Does the charm already exist in juju? If so use it, rather than
//     attempting downloading.
//   - Check endpoint bindings against existing
//   - Subordinates may not have constraints nor numunits specified
//   - Supplied charm config must validate against config defined in the charm.
//   - Check charm assumptions against the controller config, defined in core
//     assumes featureset.
//   - Check minimum juju version against current as defined in charm.
//   - NumUnits must be 1 if AttachedStorage used
//   - CharmOrigin validation, see common.ValidateCharmOrigin
//   - Manual deploy of juju-controller charm not allowed.
//
// IAAS specific (see iaasDeployFromRepositoryValidator)
// CAAS specific (see caasDeployFromRepositoryValidator)
//
// validateDeployFromRepositoryArgs does validation of all provided
// arguments. Returned is a deployTemplate which contains validated
// data necessary to deploy the application.
// Where possible, errors will be grouped and returned as a list.
func (v *deployFromRepositoryValidator) validate(ctx context.Context, arg params.DeployFromRepositoryArg) (deployTemplate, []error) {
	errs := make([]error, 0)

	if err := checkMachinePlacement(v.state, v.modelInfo.UUID, arg.ApplicationName, arg.Placement); err != nil {
		errs = append(errs, err)
	}

	// get the charm data to validate against, either a previously deployed
	// charm or the essential metadata from a charm to be async downloaded.
	charmResult, err := v.getCharm(ctx, arg)
	if err != nil {
		// return any errors here, there is no need to continue with
		// validation if we cannot find the charm.
		return deployTemplate{}, append(errs, err)
	}

	// Various checks of the resolved charm against the arg provided.
	dt, rcErrs := v.resolvedCharmValidation(ctx, charmResult.Charm, arg)
	if len(rcErrs) > 0 {
		errs = append(errs, rcErrs...)
	}

	dt.charmURL = charmResult.CharmURL
	dt.dryRun = arg.DryRun
	dt.force = arg.Force
	dt.origin = charmResult.Origin
	dt.placement = arg.Placement
	dt.storage = arg.Storage
	if len(arg.EndpointBindings) > 0 {
		bindings, err := v.newStateBindings(v.state, arg.EndpointBindings)
		if err != nil {
			errs = append(errs, err)
		} else {
			dt.endpoints = bindings.Map()
		}
	}

	// Resolve resources and validate against the charm metadata.
	resources, resourcesToUpload, resolveResErr := v.resolveResources(ctx, dt.charmURL, dt.origin, dt.resources, charmResult.Charm.Meta().Resources)
	if resolveResErr != nil {
		errs = append(errs, resolveResErr)
	}
	dt.resourcesToUpload = resourcesToUpload
	dt.resolvedResources = resources

	dt.downloadInfo = charmResult.DownloadInfo

	if v.logger.IsLevelEnabled(corelogger.TRACE) {
		v.logger.Tracef(context.TODO(), "validateDeployFromRepositoryArgs returning: %s", pretty.Sprint(dt))
	}
	return dt, errs
}

func validateAndParseAttachStorage(input []string, numUnits int) ([]names.StorageTag, []error) {
	// Parse storage tags in AttachStorage.
	if len(input) > 0 && numUnits != 1 {
		return nil, []error{errors.Errorf("AttachStorage is non-empty, but NumUnits is %d", numUnits)}
	}
	if len(input) == 0 {
		return nil, nil
	}
	attachStorage := make([]names.StorageTag, len(input))
	errs := make([]error, 0)
	for i, stor := range input {
		if names.IsValidStorage(stor) {
			attachStorage[i] = names.NewStorageTag(stor)
		} else {
			errs = append(errs, errors.NotValidf("storage name %q", stor))
		}
	}
	return attachStorage, errs
}

func (v *deployFromRepositoryValidator) resolvedCharmValidation(ctx context.Context, resolvedCharm charm.Charm, arg params.DeployFromRepositoryArg) (deployTemplate, []error) {
	errs := make([]error, 0)

	var cons constraints.Value
	var numUnits int
	if resolvedCharm.Meta().Subordinate {
		if arg.NumUnits != nil && *arg.NumUnits != 0 && constraints.IsEmpty(&arg.Cons) {
			numUnits = 0
		}
		if !constraints.IsEmpty(&arg.Cons) {
			errs = append(errs, fmt.Errorf("subordinate application must be deployed without constraints"))
		}
	} else {
		cons = arg.Cons

		if arg.NumUnits != nil {
			numUnits = *arg.NumUnits
		} else {
			// The juju client defaults num units to 1. Ensure that a
			// charm deployed by any client has at least one if no
			// number provided.
			numUnits = 1
		}
	}

	// appNameForConfig is the application name used in a config file.
	// It is based on user knowledge and either the charm or application
	// name from the cli.
	appNameForConfig := arg.CharmName
	if arg.ApplicationName != "" {
		appNameForConfig = arg.ApplicationName
	}
	appConfig, settings, err := v.appCharmSettings(appNameForConfig, arg.Trust, resolvedCharm.Config(), arg.ConfigYAML)
	if err != nil {
		errs = append(errs, err)
	}

	if err := jujuversion.CheckJujuMinVersion(resolvedCharm.Meta().MinJujuVersion, jujuversion.Current); err != nil {
		errs = append(errs, err)
	}

	// The appName is subtly different from the application config name.
	// The charm name in the metadata can be different from the charm
	// name used to deploy a charm.
	appName := resolvedCharm.Meta().Name
	if arg.ApplicationName != "" {
		appName = arg.ApplicationName
	}

	// Enforce "assumes" requirements if the feature flag is enabled.
	if err := assertCharmAssumptions(ctx, v.applicationService, resolvedCharm.Meta().Assumes); err != nil {
		if !errors.Is(err, errors.NotSupported) || !arg.Force {
			errs = append(errs, err)
		}
		v.logger.Warningf(context.TODO(), "proceeding with deployment of application even though the charm feature requirements could not be met as --force was specified")
	}

	dt := deployTemplate{
		applicationConfig: appConfig,
		applicationName:   appName,
		charm:             resolvedCharm,
		charmSettings:     settings,
		constraints:       cons,
		numUnits:          numUnits,
		resources:         arg.Resources,
	}

	return dt, errs
}

type caasDeployFromRepositoryValidator struct {
	validator *deployFromRepositoryValidator

	caasBroker     CaasBrokerInterface
	registry       storage.ProviderRegistry
	storageService StorageService

	// Needed for testing. caasDeployTemplate precheck functionality tested
	// elsewhere
	caasPrecheckFunc func(deployTemplate) error
}

// ValidateArg performs CAAS specific validation of arguments to deploy a charm
//   - Storage is not allowed
//   - Only 1 value placement allowed
//   - Block storage is not allowed
//   - Check the ServiceTypeConfigKey value is valid and find a translation
//     of types
//   - Check kubernetes model config values against the kubernetes cluster
//     in use
//   - Check the charm's min version against the caasVersion
func (v caasDeployFromRepositoryValidator) ValidateArg(ctx context.Context, arg params.DeployFromRepositoryArg) (deployTemplate, []error) {
	dt, errs := v.validator.validate(ctx, arg)
	if len(errs) > 0 {
		return dt, errs
	}
	if charm.MetaFormat(dt.charm) == charm.FormatV1 {
		errs = append(errs, errors.NotSupportedf("deploying format v1 charm %q", arg.CharmName))
	}
	// TODO
	// Convert dt.applicationConfig from Config to a map[string]string.
	// Config across the wire as a map[string]string no longer exists for
	// deploy. How to get the caas provider config here?
	if err := v.caasPrecheckFunc(dt); err != nil {
		errs = append(errs, err)
	}
	return dt, errs
}

type iaasDeployFromRepositoryValidator struct {
	validator *deployFromRepositoryValidator
}

// ValidateArg validates DeployFromRepositoryArg from an iaas perspective.
// First checking the common validation, then any validation specific to
// iaas charms.
func (v iaasDeployFromRepositoryValidator) ValidateArg(ctx context.Context, arg params.DeployFromRepositoryArg) (deployTemplate, []error) {
	dt, errs := v.validator.validate(ctx, arg)
	if len(errs) > 0 {
		return dt, errs
	}
	attachStorage, attachStorageErrs := validateAndParseAttachStorage(arg.AttachStorage, dt.numUnits)
	if len(attachStorageErrs) > 0 {
		errs = append(errs, attachStorageErrs...)
	}
	dt.attachStorage = attachStorage
	return dt, errs
}

func (v *deployFromRepositoryValidator) createOrigin(ctx context.Context, arg params.DeployFromRepositoryArg) (*charm.URL, corecharm.Origin, bool, error) {
	path, err := charm.EnsureSchema(arg.CharmName, charm.CharmHub)
	if err != nil {
		return nil, corecharm.Origin{}, false, err
	}
	curl, err := charm.ParseURL(path)
	if err != nil {
		return nil, corecharm.Origin{}, false, err
	}
	if arg.Revision != nil {
		curl = curl.WithRevision(*arg.Revision)
	}
	if !charm.CharmHub.Matches(curl.Schema) {
		return nil, corecharm.Origin{}, false, errors.Errorf("unknown schema for charm URL %q", curl.String())
	}
	channelStr := corecharm.DefaultChannelString
	if arg.Channel != nil && *arg.Channel != "" {
		channelStr = *arg.Channel
	}
	channel, err := charm.ParseChannelNormalize(channelStr)
	if err != nil {
		return nil, corecharm.Origin{}, false, err
	}

	plat, usedModelDefaultBase, err := v.deducePlatform(ctx, arg)
	if err != nil {
		return nil, corecharm.Origin{}, false, err
	}

	origin := corecharm.Origin{
		Channel:  &channel,
		Platform: plat,
		Revision: arg.Revision,
		Source:   corecharm.CharmHub,
	}
	return curl, origin, usedModelDefaultBase, nil
}

// deducePlatform returns a platform for initial resolveCharm call.
// At minimum, it must contain an architecture.
// Platform is determined by the args: architecture constraint and provided
// base. Or from the model default architecture and base.
// - If no base provided, use model default base.
// - If no model default base, will be determined later.
// - If no architecture provided, use model default. Fallback
// to DefaultArchitecture.
//
// Then check for the platform of any machine scoped placement directives.
// Use that for the platform if no base provided by the user.
// Return an error if the placement platform and user provided base do not
// match.
func (v *deployFromRepositoryValidator) deducePlatform(ctx context.Context, arg params.DeployFromRepositoryArg) (corecharm.Platform, bool, error) {
	argArch := arg.Cons.Arch
	argBase := arg.Base
	var usedModelDefaultBase bool

	// Try argBase with provided argArch and argBase first.
	platform := corecharm.Platform{}
	if argArch != nil {
		platform.Architecture = *argArch
	}
	// Fallback to model defaults if set. DefaultArchitecture otherwise.
	if platform.Architecture == "" {
		// TODO(CodingCookieRookie): Add logic for getting arch from model constraints
		// and setting as platform arch
		platform.Architecture = arch.DefaultArchitecture
	}
	if argBase != nil {
		base, err := corebase.ParseBase(argBase.Name, argBase.Channel)
		if err != nil {
			return corecharm.Platform{}, usedModelDefaultBase, err
		}
		platform.OS = base.OS
		platform.Channel = base.Channel.String()
	}

	// Initial validation of platform from known data.
	_, err := corecharm.ParsePlatform(platform.String())
	if err != nil && !errors.Is(err, errors.BadRequest) {
		return corecharm.Platform{}, usedModelDefaultBase, err
	}

	placementPlatform, placementsMatch, err := v.platformFromPlacement(ctx, arg.Placement)
	if err != nil {
		return corecharm.Platform{}, usedModelDefaultBase, err
	}
	// No machine scoped placement to match, return after checking
	// if using default model base.
	if placementPlatform == nil {
		return v.modelDefaultBase(ctx, platform)
	}
	// There can be only 1 platform.
	if !placementsMatch {
		return corecharm.Platform{}, usedModelDefaultBase, errors.BadRequestf("bases of existing placement machines do not match each other")
	}

	// No base args provided. Use the placement platform to deploy.
	if argBase == nil {
		v.logger.Tracef(context.TODO(), "using placement platform %q to deploy", placementPlatform.String())
		return *placementPlatform, usedModelDefaultBase, nil
	}

	// Check that the placement platform and the derived platform match
	// when a base is supplied. There is no guarantee that all placement
	// directives are machine scoped.
	if placementPlatform.String() == platform.String() {
		return *placementPlatform, usedModelDefaultBase, nil
	}

	if usedModelDefaultBase {
		err = fmt.Errorf("base from placements, %q, does not match model default base %q", placementPlatform.String(), platform.String())
	} else {
		err = fmt.Errorf("base from placements, %q, does not match requested base %q", placementPlatform.String(), platform.String())
	}
	return corecharm.Platform{}, usedModelDefaultBase, err

}

func (v *deployFromRepositoryValidator) modelDefaultBase(ctx context.Context, p corecharm.Platform) (corecharm.Platform, bool, error) {
	// No provided platform channel, check model defaults.
	if p.Channel != "" {
		return p, false, nil
	}
	mCfg, err := v.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return p, false, nil
	}
	db, ok := mCfg.DefaultBase()
	if !ok {
		return p, false, nil
	}
	defaultBase, err := corebase.ParseBaseFromString(db)
	if err != nil {
		return corecharm.Platform{}, false, err
	}
	p.OS = defaultBase.OS
	p.Channel = defaultBase.Channel.String()
	return p, true, nil
}

// platformFromPlacement attempts to choose a platform to deploy with based on the
// machine scoped placement values provided by the user. The platform for all provided
// machines much match.
func (v *deployFromRepositoryValidator) platformFromPlacement(ctx context.Context, placements []*instance.Placement) (*corecharm.Platform, bool, error) {
	if len(placements) == 0 {
		return nil, false, nil
	}

	machines := make([]Machine, 0)
	var machineScopeCnt int
	// Find which machines in placement actually exist.
	for _, placement := range placements {
		if placement.Scope != instance.MachineScope {
			continue
		}
		machineScopeCnt += 1
		m, err := v.state.Machine(placement.Directive)
		if err != nil {
			return nil, false, errors.Annotate(err, "verifying machine for placement")
		}
		machines = append(machines, m)
	}

	if machineScopeCnt == 0 {
		// Not all placements refer to actual machines, no need to continue.
		v.logger.Tracef(context.TODO(), "no machine scoped directives found in placements")
		return nil, false, nil
	}

	// Gather platforms for existing machines
	var platform corecharm.Platform
	// Use a set to determine if all the machines have the same platform.
	platStrings := set.NewStrings()
	for _, machine := range machines {
		b := machine.Base()
		machineUUID, err := v.machineService.GetMachineUUID(ctx, coremachine.Name(machine.Id()))
		if err != nil {
			if errors.Is(err, machineerrors.MachineNotFound) {
				return nil, false, fmt.Errorf("machine %q not started, please retry when started", machine.Id())
			}
			return nil, false, err
		}
		hc, err := v.machineService.HardwareCharacteristics(ctx, machineUUID)
		if err != nil {
			return nil, false, err
		}
		mArch := hc.Arch
		if mArch == nil {
			return nil, false, fmt.Errorf("machine %q has no saved architecture", machine.Id())
		}
		platString := fmt.Sprintf("%s/%s/%s", *mArch, b.OS, b.Channel)
		p, err := corecharm.ParsePlatformNormalize(platString)
		if err != nil {
			return nil, false, err
		}
		platform = p
		platStrings.Add(p.String())
	}
	if platStrings.Size() != 1 {
		v.logger.Errorf(context.TODO(), "mismatched platforms for machine scoped placements %s", platStrings.SortedValues())
	}

	return &platform, platStrings.Size() == 1, nil
}

func (v *deployFromRepositoryValidator) resolveCharm(ctx context.Context, curl *charm.URL, requestedOrigin corecharm.Origin, force, usedModelDefaultBase bool, cons constraints.Value) (corecharm.ResolvedDataForDeploy, error) {
	repo, err := v.getCharmRepository(ctx)
	if err != nil {
		return corecharm.ResolvedDataForDeploy{}, errors.Trace(err)
	}

	// TODO (hml) 2023-05-16
	// Use resource data found in resolvedData as part of ResolveResource.
	// Will require a new method on the repo.
	resolvedData, resolveErr := repo.ResolveForDeploy(ctx, corecharm.CharmID{URL: curl, Origin: requestedOrigin})
	if corecharm.IsUnsupportedBaseError(resolveErr) {
		if !force {
			msg := fmt.Sprintf("%v. Use --force to deploy the charm anyway.", resolveErr)
			if usedModelDefaultBase {
				msg += " Used the default-base."
			}
			return corecharm.ResolvedDataForDeploy{}, errors.New(msg)
		}
	} else if resolveErr != nil {
		return corecharm.ResolvedDataForDeploy{}, errors.Trace(resolveErr)
	}
	resolvedOrigin := &resolvedData.EssentialMetadata.ResolvedOrigin

	// TODO(CodingCookieRookie): Retrieve model constraints to be used later

	// The charmhub API can return "all" for architecture as it's not a real
	// arch we don't know how to correctly model it. "all " doesn't mean use the
	// default arch, it means use any arch which isn't quite the same. So if we
	// do get "all" we should see if there is a clean way to resolve it.
	if resolvedOrigin.Platform.Architecture == "all" {
		// TODO(CodingCookieRookie): Retrieve the model constraint architecture from dqlite and use it
		// as the first arg in constraints.ArchOrDefault instead of DefaultArchitecture
		resolvedOrigin.Platform.Architecture = arch.DefaultArchitecture
	}

	var requestedBase corebase.Base
	if requestedOrigin.Platform.OS != "" {
		// The requested base has either been specified directly as a
		// base argument, or via model config DefaultBase, to be
		// part of the requestedOrigin.
		var err error
		requestedBase, err = corebase.ParseBase(requestedOrigin.Platform.OS, requestedOrigin.Platform.Channel)
		if err != nil {
			return corecharm.ResolvedDataForDeploy{}, errors.Trace(err)
		}
	}

	modelCfg, err := v.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return corecharm.ResolvedDataForDeploy{}, errors.Trace(err)
	}
	supportedBases, err := corebase.ParseManifestBases(resolvedData.EssentialMetadata.Manifest.Bases)
	if err != nil {
		return corecharm.ResolvedDataForDeploy{}, errors.Trace(err)
	}
	bsCfg := corecharm.SelectorConfig{
		Config:              modelCfg,
		Force:               force,
		Logger:              v.logger,
		RequestedBase:       requestedBase,
		SupportedCharmBases: supportedBases,
		WorkloadBases:       corebase.WorkloadBases(),
		// TODO(CodingCookieRookie): Refactor UsingImageID to depend on model constraints HasImageID attribute as well
		UsingImageID: cons.HasImageID(),
	}
	selector, err := corecharm.ConfigureBaseSelector(bsCfg)
	if err != nil {
		return corecharm.ResolvedDataForDeploy{}, errors.Trace(err)
	}
	// Get the base to use.
	base, err := selector.CharmBase()
	if corecharm.IsUnsupportedBaseError(err) {
		msg := fmt.Sprintf("%v. Use --force to deploy the charm anyway.", err)
		if usedModelDefaultBase {
			msg += " Used the default-base."
		}
		return corecharm.ResolvedDataForDeploy{}, errors.New(msg)
	} else if err != nil {
		return corecharm.ResolvedDataForDeploy{}, errors.Trace(err)
	}
	v.logger.Tracef(context.TODO(), "Using base %q from %v to deploy %v", base, supportedBases, curl)

	resolvedOrigin.Platform.OS = base.OS
	// Avoid using Channel.String() here instead of Channel.Track for the Platform.Channel,
	// because String() will return "track/risk" if the channel's risk is non-empty
	resolvedOrigin.Platform.Channel = base.Channel.Track

	return resolvedData, nil
}

type charmResult struct {
	CharmURL     *charm.URL
	Origin       corecharm.Origin
	Charm        charm.Charm
	DownloadInfo corecharm.DownloadInfo
}

// getCharm returns the charm being deployed. Either it already has been
// used once, and we get the data from state. Or we get the essential metadata.
func (v *deployFromRepositoryValidator) getCharm(ctx context.Context, arg params.DeployFromRepositoryArg) (charmResult, error) {
	initialCurl, requestedOrigin, usedModelDefaultBase, err := v.createOrigin(ctx, arg)
	if err != nil {
		return charmResult{}, errors.Trace(err)
	}
	v.logger.Tracef(context.TODO(), "from createOrigin: %s, %s", initialCurl, pretty.Sprint(requestedOrigin))

	// Fetch the essential metadata that we require to deploy the charm
	// without downloading the full archive. The remaining metadata will
	// be populated once the charm gets downloaded.
	resolvedData, err := v.resolveCharm(ctx, initialCurl, requestedOrigin, arg.Force, usedModelDefaultBase, arg.Cons)
	if err != nil {
		return charmResult{}, errors.Trace(err)
	}
	essentialMetadata := resolvedData.EssentialMetadata
	resolvedOrigin := essentialMetadata.ResolvedOrigin
	v.logger.Tracef(context.TODO(), "from resolveCharm: %s, %s", resolvedData.URL, pretty.Sprint(resolvedOrigin))
	if resolvedOrigin.Type != "charm" {
		return charmResult{}, errors.BadRequestf("%q is not a charm", arg.CharmName)
	}

	resolvedCharm := corecharm.NewCharmInfoAdaptor(essentialMetadata)
	if resolvedCharm.Meta().Name == bootstrap.ControllerCharmName {
		return charmResult{}, errors.NotSupportedf("manual deploy of the controller charm")
	}

	// Check if a charm already exists for this charm URL. If so, the
	// charm has already been queued for download so this is a no-op. We
	// still need to resolve and return back a suitable origin as charmhub
	// may refer to the same blob using the same revision in different
	// channels.
	charmSource, err := applicationcharm.ParseCharmSchema(charm.Schema(resolvedData.URL.Schema))
	if err != nil {
		return charmResult{}, errors.Trace(err)
	}
	deployedCharm, _, _, err := v.applicationService.GetCharm(ctx, applicationcharm.CharmLocator{
		Name:     resolvedData.URL.Name,
		Revision: resolvedData.URL.Revision,
		Source:   charmSource,
	})
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return charmResult{
			CharmURL:     resolvedData.URL,
			Origin:       resolvedOrigin,
			Charm:        resolvedCharm,
			DownloadInfo: essentialMetadata.DownloadInfo,
		}, nil
	} else if err != nil {
		return charmResult{}, errors.Trace(err)
	}
	return charmResult{
		CharmURL:     resolvedData.URL,
		Origin:       resolvedOrigin,
		Charm:        deployedCharm,
		DownloadInfo: essentialMetadata.DownloadInfo,
	}, nil

}

func (v *deployFromRepositoryValidator) appCharmSettings(appName string, trust bool, chCfg *charm.Config, configYAML string) (*config.Config, charm.Settings, error) {
	if !trust && configYAML == "" {
		return nil, nil, nil
	}
	// Cheat with trust. Trust is passed to DeployFromRepository as a flag, however
	// it's handled internally to juju as an application config. As DFR only
	// has charm config via yaml, stick trust into the config via map to enable
	// reuse of current parseCharmSettings as used with the old deploy and
	// setConfig.
	// At deploy time, there's no need to include "trust=false" as missing is the same thing.
	var cfg map[string]string
	if trust {
		cfg = map[string]string{"trust": "true"}
	}
	appConfig, _, charmSettings, _, err := parseCharmSettings(chCfg, appName, cfg, configYAML, environsconfig.NoDefaults)
	return appConfig, charmSettings, err
}

func (v *deployFromRepositoryValidator) getCharmRepository(ctx context.Context) (corecharm.Repository, error) {
	modelCfg, err := v.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	charmhubURL, _ := modelCfg.CharmHubURL()

	return v.newCharmHubRepository(repository.CharmHubRepositoryConfig{
		Logger:             v.logger,
		CharmhubHTTPClient: v.charmhubHTTPClient,
		CharmhubURL:        charmhubURL,
	})
}
