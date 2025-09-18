// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/schema"
	"gopkg.in/macaroon.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	apiservercharms "github.com/juju/juju/apiserver/internal/charms"
	coreapplication "github.com/juju/juju/core/application"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/leadership"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/permission"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/domain/resolve"
	resolveerrors "github.com/juju/juju/domain/resolve/errors"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/configschema"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

// APIv22 provides the Application API facade for version 22.
type APIv22 struct {
	*APIBase
}

// APIv21 provides the Application API facade for version 21.
type APIv21 struct {
	*APIv22
}

// APIv20 provides the Application API facade for version 20.
type APIv20 struct {
	*APIv21
}

// APIv19 provides the Application API facade for version 19.
type APIv19 struct {
	*APIv20
}

// APIBase implements the shared application interface and is the concrete
// implementation of the api end point.
type APIBase struct {
	store objectstore.ObjectStore

	authorizer facade.Authorizer
	check      BlockChecker
	repoDeploy DeployFromRepository

	controllerUUID            string
	modelUUID                 model.UUID
	modelType                 model.ModelType
	modelConfigService        ModelConfigService
	machineService            MachineService
	applicationService        ApplicationService
	resolveService            ResolveService
	networkService            NetworkService
	portService               PortService
	relationService           RelationService
	removalService            RemovalService
	resourceService           ResourceService
	storageService            StorageService
	externalControllerService ExternalControllerService
	crossModelRelationService CrossModelRelationService

	leadershipReader leadership.Reader

	registry              storage.ProviderRegistry
	caasBroker            CaasBrokerInterface
	deployApplicationFunc DeployApplicationFunc

	logger corelogger.Logger
	clock  clock.Clock
}

type CaasBrokerInterface interface {
	ValidateStorageClass(ctx context.Context, config map[string]interface{}) error
}

func newFacadeBase(stdCtx context.Context, ctx facade.ModelContext) (*APIBase, error) {
	domainServices := ctx.DomainServices()
	blockChecker := common.NewBlockChecker(domainServices.BlockCommand())

	storageService := domainServices.Storage()

	registry, err := storageService.GetStorageRegistry(stdCtx)
	if err != nil {
		return nil, errors.Annotate(err, "getting storage registry")
	}

	modelInfo, err := domainServices.ModelInfo().GetModelInfo(stdCtx)
	if err != nil {
		return nil, internalerrors.Errorf("getting model info: %w", err)
	}

	leadershipReader, err := ctx.LeadershipReader()
	if err != nil {
		return nil, errors.Trace(err)
	}

	charmhubHTTPClient, err := ctx.HTTPClient(corehttp.CharmhubPurpose)
	if err != nil {
		return nil, fmt.Errorf(
			"getting charm hub http client: %w",
			err,
		)
	}

	repoLogger := ctx.Logger().Child("deployfromrepo")

	applicationService := domainServices.Application()

	validatorCfg := validatorConfig{
		charmhubHTTPClient: charmhubHTTPClient,
		caasBroker:         nil,
		modelInfo:          modelInfo,
		modelConfigService: domainServices.Config(),
		machineService:     domainServices.Machine(),
		applicationService: applicationService,
		registry:           registry,
		storageService:     storageService,
		logger:             repoLogger,
	}

	repoDeploy := NewDeployFromRepositoryAPI(
		modelInfo.Type,
		applicationService,
		ctx.ObjectStore(),
		makeDeployFromRepositoryValidator(stdCtx, validatorCfg),
		repoLogger,
		ctx.Clock(),
	)

	return NewAPIBase(
		Services{
			ExternalControllerService: domainServices.ExternalController(),
			NetworkService:            domainServices.Network(),
			ModelConfigService:        domainServices.Config(),
			MachineService:            domainServices.Machine(),
			ApplicationService:        applicationService,
			ResolveService:            domainServices.Resolve(),
			PortService:               domainServices.Port(),
			RelationService:           domainServices.Relation(),
			RemovalService:            domainServices.Removal(),
			ResourceService:           domainServices.Resource(),
			StorageService:            storageService,
			CrossModelRelationService: domainServices.CrossModelRelation(),
		},
		ctx.Auth(),
		blockChecker,
		ctx.ControllerUUID(),
		modelInfo.UUID,
		modelInfo.Type,
		leadershipReader,
		repoDeploy,
		DeployApplication,
		registry,
		nil,
		ctx.ObjectStore(),
		ctx.Logger().Child("application"),
		ctx.Clock(),
	)
}

// DeployApplicationFunc is a function that deploys an application.
type DeployApplicationFunc = func(
	context.Context,
	model.ModelType,
	ApplicationService,
	StorageService,
	objectstore.ObjectStore,
	DeployApplicationParams,
	corelogger.Logger,
	clock.Clock,
) error

// NewAPIBase returns a new application API facade.
func NewAPIBase(
	services Services,
	authorizer facade.Authorizer,
	blockChecker BlockChecker,
	controllerUUID string,
	modelUUID model.UUID,
	modelType model.ModelType,
	leadershipReader Leadership,
	repoDeploy DeployFromRepository,
	deployApplication DeployApplicationFunc,
	registry storage.ProviderRegistry,
	caasBroker CaasBrokerInterface,
	store objectstore.ObjectStore,
	logger corelogger.Logger,
	clock clock.Clock,
) (*APIBase, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	if err := services.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	return &APIBase{
		authorizer:            authorizer,
		repoDeploy:            repoDeploy,
		check:                 blockChecker,
		controllerUUID:        controllerUUID,
		modelUUID:             modelUUID,
		modelType:             modelType,
		leadershipReader:      leadershipReader,
		deployApplicationFunc: deployApplication,
		registry:              registry,
		caasBroker:            caasBroker,
		store:                 store,

		externalControllerService: services.ExternalControllerService,
		applicationService:        services.ApplicationService,
		resolveService:            services.ResolveService,
		machineService:            services.MachineService,
		modelConfigService:        services.ModelConfigService,
		networkService:            services.NetworkService,
		portService:               services.PortService,
		relationService:           services.RelationService,
		removalService:            services.RemovalService,
		resourceService:           services.ResourceService,
		storageService:            services.StorageService,
		crossModelRelationService: services.CrossModelRelationService,

		logger: logger,
		clock:  clock,
	}, nil
}

// checkAccess checks if this API has the requested access level.
func (api *APIBase) checkAccess(ctx context.Context, access permission.Access) error {
	return api.authorizer.HasPermission(ctx, access, names.NewModelTag(api.modelUUID.String()))
}

func (api *APIBase) checkCanRead(ctx context.Context) error {
	return api.checkAccess(ctx, permission.ReadAccess)
}

func (api *APIBase) checkCanWrite(ctx context.Context) error {
	return api.checkAccess(ctx, permission.WriteAccess)
}

// Deploy fetches the charms from the charm store and deploys them
// using the specified placement directives.
func (api *APIv20) Deploy(ctx context.Context, args params.ApplicationsDeploy) (params.ErrorResults, error) {
	// APIv20 does not support attach storage.
	for _, appArgs := range args.Applications {
		if len(appArgs.AttachStorage) > 0 {
			return params.ErrorResults{}, errors.Errorf(
				"AttachStorage may not be specified for container models",
			)
		}
	}
	return api.APIv21.Deploy(ctx, args)
}

// Deploy fetches the charms from the charm store and deploys them
// using the specified placement directives.
func (api *APIBase) Deploy(ctx context.Context, args params.ApplicationsDeploy) (params.ErrorResults, error) {
	if err := api.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Applications)),
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return result, errors.Trace(err)
	}

	for i, arg := range args.Applications {
		if err := apiservercharms.ValidateCharmOrigin(arg.CharmOrigin); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// Fill in the charm origin revision from the charm url if it's absent
		if arg.CharmOrigin.Revision == nil {
			curl, err := charm.ParseURL(arg.CharmURL)
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			rev := curl.Revision
			arg.CharmOrigin.Revision = &rev
		}
		err := api.deployApplication(ctx, arg)
		if err == nil {
			// Deploy succeeded, no cleanup needed, move on to the next.
			continue
		}
		result.Results[i].Error = apiservererrors.ServerError(errors.Annotatef(err, "cannot deploy %q", arg.ApplicationName))

		api.cleanupResourcesAddedBeforeApp(ctx, arg.ApplicationName, arg.Resources)
	}
	return result, nil
}

// cleanupResourcesAddedBeforeApp deletes any resources added before the
// application. Errors will be logged but not reported to the user. These
// errors mask the real deployment failure.
func (api *APIBase) cleanupResourcesAddedBeforeApp(ctx context.Context, appName string, argResources map[string]string) {
	if len(argResources) == 0 {
		return
	}

	pendingIDs := make([]coreresource.UUID, 0, len(argResources))
	for _, resource := range argResources {
		resUUID, err := coreresource.ParseUUID(resource)
		if err != nil {
			api.logger.Warningf(ctx, "unable to parse resource UUID %q, while cleaning up pending"+
				" resources from a failed application deployment: %w", resource, err)
			continue
		}
		pendingIDs = append(pendingIDs, resUUID)
	}
	err := api.resourceService.DeleteResourcesAddedBeforeApplication(ctx, pendingIDs)
	if err != nil {
		api.logger.Errorf(ctx, "removing pending resources for %q: %w", appName, err)
	}
}

// ConfigSchema returns the config schema and defaults for an application.
func ConfigSchema() (configschema.Fields, schema.Defaults, error) {
	return trustFields, trustDefaults, nil
}

func splitTrustFromApplicationConfig(cfg charm.Config) (bool, charm.Config) {
	if trust, ok := cfg[coreapplication.TrustConfigOptionName]; ok {
		delete(cfg, coreapplication.TrustConfigOptionName)
		switch t := trust.(type) {
		case bool:
			return t, cfg
		case string:
			trustBool, err := strconv.ParseBool(t)
			if err != nil {
				return false, cfg
			}
			return trustBool, cfg
		default:
			return false, cfg
		}
	} else {
		return false, cfg
	}
}

// splitTrustFromApplicationConfigFromYAML splits the trust flag from the application config
// from the YAML string.
//
// The YAML config can be formatted in two ways:
//  1. The YAML config is a set of key-value pairs corresponding to application
//     config/settings (i.e. can include trust).
//  2. The YAML config is a set of key-value pairs corresponding to application
//     config/settings, indexed under the application name.
func splitTrustFromApplicationConfigFromYAML(inYaml, appName string) (
	trust bool,
	applicationConfig charm.Config,
	_ error,
) {
	var allSettings map[string]interface{}
	if err := goyaml.Unmarshal([]byte(inYaml), &allSettings); err != nil {
		return false, nil, errors.Annotate(err, "cannot parse settings data")
	}

	if val, ok := allSettings[appName].(map[interface{}]interface{}); ok {
		subSettings := make(map[string]interface{})
		for k, v := range val {
			strK, ok := k.(string)
			if !ok {
				return false, nil, errors.Errorf("config key %q has invalid type %T", k, k)
			}
			subSettings[strK] = v
		}
		trust, applicationConfig = splitTrustFromApplicationConfig(subSettings)
		return trust, applicationConfig, nil
	}

	trust, applicationConfig = splitTrustFromApplicationConfig(allSettings)
	return trust, applicationConfig, nil
}

// caasDeployParams contains deploy configuration requiring prechecks
// specific to a caas.
type caasDeployParams struct {
	applicationName string
	attachStorage   []string
	charm           CharmMeta
	placement       []*instance.Placement
}

// precheck, checks the deploy config based on caas specific
// requirements.
func (c caasDeployParams) precheck(
	ctx context.Context,
	modelConfigService ModelConfigService,
	storageService StorageService,
	registry storage.ProviderRegistry,
	caasBroker CaasBrokerInterface,
) error {
	if len(c.placement) > 1 {
		return errors.Errorf(
			"only 1 placement directive is supported for container models, got %d",
			len(c.placement),
		)
	}
	for _, s := range c.charm.Meta().Storage {
		if s.Type == charm.StorageBlock {
			return errors.Errorf("block storage %q is not supported for container charms", s.Name)
		}
	}
	return nil
}

// deployApplication fetches the charm from the charm store and deploys it.
// The logic has been factored out into a common function which is called by
// both the legacy API on the client facade, as well as the new application facade.
func (api *APIBase) deployApplication(
	ctx context.Context,
	args params.ApplicationDeploy,
) error {
	curl, err := charm.ParseURL(args.CharmURL)
	if err != nil {
		return errors.Trace(err)
	}
	if curl.Revision < 0 {
		return errors.Errorf("charm url must include revision")
	}

	// This check is done early so that errors deeper in the call-stack do not
	// leave an application deployment in an unrecoverable error state.
	if err := checkMachinePlacement(api.modelUUID, args.ApplicationName, args.Placement); err != nil {
		return errors.Trace(err)
	}

	locator, err := apiservercharms.CharmLocatorFromURL(args.CharmURL)
	if err != nil {
		return errors.Trace(err)
	}
	ch, err := api.getCharm(ctx, locator)
	if err != nil {
		return errors.Trace(err)
	}

	// If the charm specifies a unique architecture, ensure that is set in
	// the constraints. Charmhub handles any existing arch constraints.
	cons := args.Constraints
	if !cons.HasArch() {
		arches := set.NewStrings()
		for _, base := range ch.Manifest().Bases {
			for _, arch := range base.Architectures {
				arches.Add(arch)
			}
		}
		if arches.Size() == 1 {
			cons.Arch = &arches.Values()[0]
		} else {
			api.logger.Warningf(ctx, "charm supports multiple architectures, unable to determine which to deploy to")
		}
	}

	if err := jujuversion.CheckJujuMinVersion(ch.Meta().MinJujuVersion, jujuversion.Current); err != nil {
		return errors.Trace(err)
	}

	// Codify an implicit assumption for Deploy, that AddPendingResources
	// has been called first by the client. This validates that local charm
	// and bundle deployments by a client, have provided the needed resource
	// data, whether or not the user has made specific requests. This differs
	// from the DeployFromRepository expected code path where unknown resource
	// specific are filled in by the facade method.
	if len(ch.Meta().Resources) != len(args.Resources) {
		return errors.Errorf("not all pending resources for charm provided")
	}

	if api.modelType == model.CAAS {
		caas := caasDeployParams{
			applicationName: args.ApplicationName,
			attachStorage:   args.AttachStorage,
			charm:           ch,
			placement:       args.Placement,
		}
		if err := caas.precheck(ctx, api.modelConfigService, api.storageService, api.registry, api.caasBroker); err != nil {
			return errors.Trace(err)
		}
	}

	trust, applicationConfig, err := parseApplicationConfig(args.ApplicationName, args.Config, args.ConfigYAML)
	if err != nil {
		return errors.Trace(err)
	}

	// Parse storage tags in AttachStorage.
	if len(args.AttachStorage) > 0 && args.NumUnits != 1 {
		return errors.Errorf("AttachStorage is non-empty, but NumUnits is %d", args.NumUnits)
	}
	attachStorage := make([]names.StorageTag, len(args.AttachStorage))
	for i, tagString := range args.AttachStorage {
		tag, err := names.ParseStorageTag(tagString)
		if err != nil {
			return errors.Trace(err)
		}
		attachStorage[i] = tag
	}

	origin, err := convertCharmOrigin(args.CharmOrigin)
	if err != nil {
		return errors.Trace(err)
	}

	appParams := DeployApplicationParams{
		ApplicationName:   args.ApplicationName,
		Charm:             ch,
		CharmOrigin:       origin,
		NumUnits:          args.NumUnits,
		Trust:             trust,
		ApplicationConfig: applicationConfig,
		Constraints:       cons,
		Placement:         args.Placement,
		Storage:           args.Storage,
		Devices:           args.Devices,
		AttachStorage:     attachStorage,
		EndpointBindings:  transformBindings(args.EndpointBindings),
		Resources:         args.Resources,
		Force:             args.Force,
	}
	// TODO: replace model with model info/config services
	err = api.deployApplicationFunc(ctx, api.modelType, api.applicationService,
		api.storageService, api.store, appParams, api.logger, api.clock)
	return errors.Trace(err)
}

// convertCharmOrigin converts a params CharmOrigin to a core charm
// Origin. If the input origin is nil, a core charm Origin is deduced
// from the provided data. It is used in both deploying and refreshing
// charms, including from old clients which aren't charm origin aware.
// MaybeSeries is a fallback if the origin is not provided.
func convertCharmOrigin(origin *params.CharmOrigin) (corecharm.Origin, error) {
	if origin == nil {
		return corecharm.Origin{}, errors.NotValidf("nil charm origin")
	}

	originType := origin.Type
	base, err := corebase.ParseBase(origin.Base.Name, origin.Base.Channel)
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}
	platform := corecharm.Platform{
		Architecture: origin.Architecture,
		OS:           base.OS,
		Channel:      base.Channel.Track,
	}

	var track string
	if origin.Track != nil {
		track = *origin.Track
	}
	var branch string
	if origin.Branch != nil {
		branch = *origin.Branch
	}
	// We do guarantee that there will be a risk value.
	// Ignore the error, as only caused by risk as an
	// empty string.
	var channel *charm.Channel
	if ch, err := charm.MakeChannel(track, origin.Risk, branch); err == nil {
		channel = &ch
	}

	return corecharm.Origin{
		Type:     originType,
		Source:   corecharm.Source(origin.Source),
		ID:       origin.ID,
		Hash:     origin.Hash,
		Revision: origin.Revision,
		Channel:  channel,
		Platform: platform,
	}, nil
}

// parseApplicationConfig parses and combines the config settings for a charm as
// specified by the provided config map and config yaml payload. Any
// model-specific application settings will be automatically extracted and
// returned back as an *application.Config.
func parseApplicationConfig(
	appName string, cfg map[string]string, configYaml string,
) (trust bool, applicationConfig charm.Config, err error) {
	if cfg == nil && configYaml == "" {
		return false, nil, nil
	}

	trustFromMap, applicationConfigFromMap := splitTrustFromApplicationConfig(transform.Map(
		cfg,
		func(k string, v string) (string, interface{}) { return k, v }),
	)

	trustFromYAML, applicationConfigFromYAML, err := splitTrustFromApplicationConfigFromYAML(configYaml, appName)
	if err != nil {
		return false, nil, internalerrors.Errorf("parsing config: %w", err)
	}

	trust = trustFromMap || trustFromYAML

	// Entries from the config map take precedence over entries from the YAML file.
	applicationConfig = make(charm.Config)
	maps.Copy(applicationConfig, applicationConfigFromYAML)
	maps.Copy(applicationConfig, applicationConfigFromMap)

	return trust, applicationConfig, nil
}

// checkMachinePlacement does a non-exhaustive validation of any supplied
// placement directives.
// If the placement scope is for a machine, ensure that the machine exists.
// If the placement scope is model-uuid, replace it with the actual model uuid.
func checkMachinePlacement(modelID model.UUID, app string, placement []*instance.Placement) error {
	for _, p := range placement {
		if p == nil {
			continue
		}

		if p.Scope == instance.ModelScope {
			continue
		}

		dir := p.Directive

		toProvisionedMachine := p.Scope == instance.MachineScope
		if !toProvisionedMachine && dir == "" {
			continue
		}
	}

	return nil
}

// SetCharm sets the charm for a given for the application.
// The v1 args use "storage-constraints" as the storage directive attr tag.
func (api *APIv19) SetCharm(ctx context.Context, argsV1 params.ApplicationSetCharmV1) error {
	args := argsV1.ApplicationSetCharmV2
	args.StorageDirectives = argsV1.StorageDirectives
	return api.APIBase.SetCharm(ctx, args)
}

// SetCharm sets the charm for a given for the application.
func (api *APIBase) SetCharm(ctx context.Context, args params.ApplicationSetCharmV2) error {
	if err := api.checkCanWrite(ctx); err != nil {
		return err
	}

	if !args.ForceUnits {
		if err := api.check.ChangeAllowed(ctx); err != nil {
			return errors.Trace(err)
		}
	}

	if err := apiservercharms.ValidateCharmOrigin(args.CharmOrigin); err != nil {
		return err
	}

	newCharmLocator, err := apiservercharms.CharmLocatorFromURL(args.CharmURL)
	if err != nil {
		return errors.Trace(err)
	}

	err = api.applicationService.SetApplicationCharm(ctx, args.ApplicationName, newCharmLocator, application.SetCharmParams{})
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return errors.NotFoundf("application %q", args.ApplicationName)
	} else if errors.Is(err, applicationerrors.CharmNotFound) {
		return errors.NotFoundf("charm %q", args.CharmURL)
	} else if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// GetCharmURLOrigin returns the charm URL and charm origin the given
// application is running at present.
func (api *APIBase) GetCharmURLOrigin(ctx context.Context, args params.ApplicationGet) (params.CharmURLOriginResult, error) {
	if err := api.checkCanRead(ctx); err != nil {
		return params.CharmURLOriginResult{}, errors.Trace(err)
	}

	charmLocator, err := api.applicationService.GetCharmLocatorByApplicationName(ctx, args.ApplicationName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return params.CharmURLOriginResult{Error: apiservererrors.ServerError(errors.NotFoundf("application %q", args.ApplicationName))}, nil
	} else if err != nil {
		return params.CharmURLOriginResult{Error: apiservererrors.ServerError(err)}, nil
	}
	charmURL, err := apiservercharms.CharmURLFromLocator(charmLocator.Name, charmLocator)
	if err != nil {
		return params.CharmURLOriginResult{Error: apiservererrors.ServerError(err)}, nil
	}

	chOrigin, err := api.applicationService.GetApplicationCharmOrigin(ctx, args.ApplicationName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return params.CharmURLOriginResult{Error: apiservererrors.ServerError(errors.NotFoundf("application %q", args.ApplicationName))}, nil
	} else if err != nil {
		return params.CharmURLOriginResult{Error: apiservererrors.ServerError(err)}, nil
	}

	result := params.CharmURLOriginResult{URL: charmURL}
	if result.Origin, err = makeParamsCharmOrigin(chOrigin); err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	result.Origin.InstanceKey = charmhub.CreateInstanceKey(args.ApplicationName, names.NewModelTag(api.modelUUID.String()))
	return result, nil
}

func makeParamsCharmOrigin(origin corecharm.Origin) (params.CharmOrigin, error) {
	osType, err := encodeOSType(origin.Platform.OS)
	if err != nil {
		return params.CharmOrigin{}, errors.Trace(err)
	}
	retOrigin := params.CharmOrigin{
		Source:       origin.Source.String(),
		ID:           origin.ID,
		Hash:         origin.Hash,
		Revision:     origin.Revision,
		Architecture: origin.Platform.Architecture,
		Base: params.Base{
			Name:    osType,
			Channel: origin.Platform.Channel,
		},
	}
	if origin.Channel != nil {
		retOrigin.Risk = string(origin.Channel.Risk)
		if origin.Channel.Track != "" {
			retOrigin.Track = &origin.Channel.Track
		}
		if origin.Channel.Branch != "" {
			retOrigin.Branch = &origin.Channel.Branch
		}
	}
	return retOrigin, nil
}

func encodeOSType(t string) (string, error) {
	switch t {
	case ostype.Ubuntu.String():
		return corebase.UbuntuOS, nil
	default:
		return "", internalerrors.Errorf("unsupported OS type %v", t)
	}
}

// CharmRelations implements the server side of Application.CharmRelations.
func (api *APIBase) CharmRelations(ctx context.Context, p params.ApplicationCharmRelations) (params.ApplicationCharmRelationsResults, error) {
	var results params.ApplicationCharmRelationsResults
	if err := api.checkCanRead(ctx); err != nil {
		return results, errors.Trace(err)
	}

	appID, err := api.applicationService.GetApplicationIDByName(ctx, p.ApplicationName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return results, apiservererrors.ParamsErrorf(params.CodeNotFound, "application %q not found", p.ApplicationName)
	} else if err != nil {
		return results, apiservererrors.ServerError(err)
	}

	endpoints, err := api.applicationService.GetApplicationEndpointNames(ctx, appID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return results, apiservererrors.ParamsErrorf(params.CodeNotFound, "application %q not found", p.ApplicationName)
	} else if err != nil {
		return results, apiservererrors.ServerError(err)
	}

	results.CharmRelations = endpoints
	return results, nil
}

// Expose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (api *APIBase) Expose(ctx context.Context, args params.ApplicationExpose) error {
	if err := api.checkCanWrite(ctx); err != nil {
		return errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}

	// Map space names to space IDs before calling SetExposed
	mappedExposeParams, err := api.mapExposedEndpointParams(ctx, args.ExposedEndpoints)
	if err != nil {
		return apiservererrors.ServerError(err)
	}

	if err := api.applicationService.MergeExposeSettings(ctx, args.ApplicationName, mappedExposeParams); err != nil {
		return apiservererrors.ServerError(err)
	}
	return nil
}

func (api *APIBase) mapExposedEndpointParams(ctx context.Context, params map[string]params.ExposedEndpoint) (map[string]application.ExposedEndpoint, error) {
	if len(params) == 0 {
		return nil, nil
	}

	var res = make(map[string]application.ExposedEndpoint, len(params))

	spaceInfos, err := api.networkService.GetAllSpaces(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	for endpointName, exposeDetails := range params {
		mappedParam := application.ExposedEndpoint{
			ExposeToCIDRs: set.NewStrings(exposeDetails.ExposeToCIDRs...),
		}

		if len(exposeDetails.ExposeToSpaces) != 0 {
			spaceIDs := make([]string, len(exposeDetails.ExposeToSpaces))
			for i, spaceName := range exposeDetails.ExposeToSpaces {
				sp := spaceInfos.GetByName(network.SpaceName(spaceName))
				if sp == nil {
					return nil, errors.NotFoundf("space %q", spaceName)
				}

				spaceIDs[i] = sp.ID.String()
			}
			mappedParam.ExposeToSpaceIDs = set.NewStrings(spaceIDs...)
		}

		res[endpointName] = mappedParam

	}

	return res, nil
}

// Unexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (api *APIBase) Unexpose(ctx context.Context, args params.ApplicationUnexpose) error {
	if err := api.checkCanWrite(ctx); err != nil {
		return err
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}

	if err := api.applicationService.UnsetExposeSettings(ctx, args.ApplicationName, set.NewStrings(args.ExposedEndpoints...)); err != nil {
		return apiservererrors.ServerError(err)
	}
	return nil
}

// AddUnits adds a given number of units to an application.
func (api *APIBase) AddUnits(ctx context.Context, args params.AddApplicationUnits) (params.AddApplicationUnitsResults, error) {
	if api.modelType == model.CAAS {
		return params.AddApplicationUnitsResults{}, errors.NotSupportedf("adding units to a container-based model")
	}

	if err := api.checkCanWrite(ctx); err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}

	locator, err := api.getCharmLocatorByApplicationName(ctx, args.ApplicationName)
	if err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	charm, err := api.getCharm(ctx, locator)
	if err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}

	units, err := api.addApplicationUnits(ctx, args, charm.Meta())
	if err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	return params.AddApplicationUnitsResults{
		Units: transform.Slice(units, func(unit coreunit.Name) string { return unit.String() }),
	}, nil
}

// addApplicationUnits adds a given number of units to an application.
func (api *APIBase) addApplicationUnits(
	ctx context.Context, args params.AddApplicationUnits, charmMeta *charm.Meta,
) ([]coreunit.Name, error) {
	if args.NumUnits < 1 {
		return nil, errors.New("must add at least one unit")
	}

	assignUnits := true
	if api.modelType != model.IAAS {
		// In a CAAS model, there are no machines for
		// units to be assigned to.
		assignUnits = false
		if len(args.AttachStorage) > 0 {
			return nil, errors.Errorf(
				"AttachStorage may not be specified for %s models",
				api.modelType,
			)
		}
		if len(args.Placement) > 1 {
			return nil, errors.Errorf(
				"only 1 placement directive is supported for %s models, got %d",
				api.modelType,
				len(args.Placement),
			)
		}
	}

	// Parse storage tags in AttachStorage.
	if len(args.AttachStorage) > 0 && args.NumUnits != 1 {
		return nil, errors.Errorf("AttachStorage is non-empty, but NumUnits is %d", args.NumUnits)
	}
	attachStorage := make([]names.StorageTag, len(args.AttachStorage))
	for i, tagString := range args.AttachStorage {
		tag, err := names.ParseStorageTag(tagString)
		if err != nil {
			return nil, errors.Trace(err)
		}
		attachStorage[i] = tag
	}

	return api.addUnits(
		ctx,
		args.ApplicationName,
		args.NumUnits,
		args.Placement,
		attachStorage,
		assignUnits,
		charmMeta,
	)
}

// DestroyUnit removes a given set of application units.
func (api *APIBase) DestroyUnit(ctx context.Context, args params.DestroyUnitsParams) (params.DestroyUnitResults, error) {
	if api.modelType == model.CAAS {
		return params.DestroyUnitResults{}, errors.NotSupportedf("removing units on a non-container model")
	}
	if err := api.checkCanWrite(ctx); err != nil {
		return params.DestroyUnitResults{}, errors.Trace(err)
	}
	if err := api.check.RemoveAllowed(ctx); err != nil {
		return params.DestroyUnitResults{}, errors.Trace(err)
	}

	destroyUnit := func(arg params.DestroyUnitParams) (*params.DestroyUnitInfo, error) {
		unitTag, err := names.ParseUnitTag(arg.UnitTag)
		if err != nil {
			return nil, errors.Trace(err)
		}

		unitName, err := coreunit.NewName(unitTag.Id())
		if err != nil {
			return nil, internalerrors.Errorf("parsing unit name %q: %w", unitName, err)
		}
		appName := unitName.Application()

		isSubordinate, err := api.applicationService.IsSubordinateApplicationByName(ctx, appName)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			return nil, errors.NotFoundf("application %s", appName)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if isSubordinate {
			return nil, errors.Errorf("unit %q is a subordinate, to remove use remove-relation. Note: this will remove all units of %q",
				unitName, appName)
		}

		locator, err := api.getCharmLocatorByApplicationName(ctx, appName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		charmName, err := api.getCharmName(ctx, locator)
		if err != nil {
			return nil, errors.Trace(err)
		} else if charmName == bootstrap.ControllerCharmName {
			return nil, errors.NotSupportedf("removing units from the controller application")
		}

		var info params.DestroyUnitInfo

		// TODO(storage): return detached / destroyed storage volumes/filesystems

		if arg.DryRun {
			return &info, nil
		}

		unitUUID, err := api.applicationService.GetUnitUUID(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			return nil, errors.NotFoundf("unit %q", unitName)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		maxWait := time.Duration(0)
		if arg.MaxWait != nil {
			maxWait = *arg.MaxWait
		}
		_, err = api.removalService.RemoveUnit(ctx, unitUUID, arg.DestroyStorage, arg.Force, maxWait)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			return nil, errors.NotFoundf("unit %q", unitName)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		return &info, nil
	}
	results := make([]params.DestroyUnitResult, len(args.Units))
	for i, entity := range args.Units {
		info, err := destroyUnit(entity)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results[i].Info = info
	}
	return params.DestroyUnitResults{
		Results: results,
	}, nil
}

// DestroyApplication removes a given set of applications.
func (api *APIBase) DestroyApplication(ctx context.Context, args params.DestroyApplicationsParams) (params.DestroyApplicationResults, error) {
	if err := api.checkCanWrite(ctx); err != nil {
		return params.DestroyApplicationResults{}, err
	}
	if err := api.check.RemoveAllowed(ctx); err != nil {
		return params.DestroyApplicationResults{}, errors.Trace(err)
	}
	destroyApp := func(arg params.DestroyApplicationParams) (*params.DestroyApplicationInfo, error) {
		tag, err := names.ParseApplicationTag(arg.ApplicationTag)
		if err != nil {
			return nil, err
		}

		locator, err := api.getCharmLocatorByApplicationName(ctx, tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}

		name, err := api.getCharmName(ctx, locator)
		if err != nil {
			return nil, errors.Trace(err)
		} else if name == bootstrap.ControllerCharmName {
			return nil, errors.NotSupportedf("removing the controller application")
		}

		unitNames, err := api.applicationService.GetUnitNamesForApplication(ctx, tag.Id())
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			return nil, errors.NotFoundf("application %q", tag.Id())
		} else if err != nil {
			return nil, errors.Trace(err)
		}

		var info params.DestroyApplicationInfo
		for _, unitName := range unitNames {
			unitTag := names.NewUnitTag(unitName.String())
			info.DestroyedUnits = append(
				info.DestroyedUnits,
				params.Entity{Tag: unitTag.String()},
			)

			// TODO(storage): return detached / destroyed storage volumes/filesystems
		}

		if arg.DryRun {
			return &info, nil
		}

		appID, err := api.applicationService.GetApplicationIDByName(ctx, tag.Id())
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			return &info, err
		} else if err != nil {
			return nil, errors.Annotatef(err, "getting application ID %q", tag.Id())
		}
		maxWait := time.Duration(0)
		if arg.MaxWait != nil {
			maxWait = *arg.MaxWait
		}
		_, err = api.removalService.RemoveApplication(ctx, appID, arg.DestroyStorage, arg.Force, maxWait)
		if err != nil && !errors.Is(err, applicationerrors.ApplicationNotFound) {
			return nil, errors.Annotatef(err, "removing application %q", tag.Id())
		}

		return &info, err
	}
	results := make([]params.DestroyApplicationResult, len(args.Applications))
	for i, arg := range args.Applications {
		info, err := destroyApp(arg)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results[i].Info = info
	}
	return params.DestroyApplicationResults{
		Results: results,
	}, nil
}

// DestroyConsumedApplications removes a given set of consumed (remote) applications.
func (api *APIBase) DestroyConsumedApplications(ctx context.Context, args params.DestroyConsumedApplicationsParams) (params.ErrorResults, error) {
	if err := api.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, err
	}
	if err := api.check.RemoveAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	results := make([]params.ErrorResult, len(args.Applications))
	for i := range args.Applications {
		results[i].Error = apiservererrors.ServerError(errors.NotImplementedf("cross model relations are disabled until " +
			"backend functionality is moved to domain"))
	}
	return params.ErrorResults{
		Results: results,
	}, nil
}

// ScaleApplications scales the specified application to the requested number of units.
func (api *APIBase) ScaleApplications(ctx context.Context, args params.ScaleApplicationsParamsV2) (params.ScaleApplicationResults, error) {
	if api.modelType != model.CAAS {
		return params.ScaleApplicationResults{}, errors.NotSupportedf("scaling applications on a non-container model")
	}
	if err := api.checkCanWrite(ctx); err != nil {
		return params.ScaleApplicationResults{}, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.ScaleApplicationResults{}, errors.Trace(err)
	}
	scaleApplication := func(arg params.ScaleApplicationParamsV2) (*params.ScaleApplicationInfo, error) {
		if arg.Scale < 0 && arg.ScaleChange == 0 {
			return nil, errors.NotValidf("scale < 0")
		} else if arg.Scale != 0 && arg.ScaleChange != 0 {
			return nil, errors.NotValidf("requesting both scale and scale-change")
		}

		appTag, err := names.ParseApplicationTag(arg.ApplicationTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		name := appTag.Id()

		storageTags, attachStorageErrs := validateAndParseAttachStorage(arg.AttachStorage, arg.ScaleChange)
		if len(attachStorageErrs) > 0 {
			var errStrings []string
			for _, err := range attachStorageErrs {
				errStrings = append(errStrings, err.Error())
			}
			return nil, errors.Errorf("failed to scale a application: %s", strings.Join(errStrings, ", "))
		}
		// TODO(storage) - implement and test attach storage for k8s
		//  update ChangeApplicationScale() below.
		if len(storageTags) > 0 {
			return nil, errors.NotImplementedf("attaching storage when scaling a k8s application")
		}

		var info params.ScaleApplicationInfo
		if arg.ScaleChange != 0 {
			newScale, err := api.applicationService.ChangeApplicationScale(ctx, name, arg.ScaleChange)
			if err != nil {
				return nil, errors.Trace(err)
			}
			info.Scale = newScale
		} else {
			if err := api.applicationService.SetApplicationScale(ctx, name, arg.Scale); err != nil {
				return nil, errors.Trace(err)
			}
			info.Scale = arg.Scale
		}
		return &info, nil
	}
	results := make([]params.ScaleApplicationResult, len(args.Applications))
	for i, entity := range args.Applications {
		info, err := scaleApplication(entity)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results[i].Info = info
	}
	return params.ScaleApplicationResults{
		Results: results,
	}, nil
}

// ScaleApplications scales the specified application to the requested number of units.
func (api *APIv20) ScaleApplications(ctx context.Context, args params.ScaleApplicationsParams) (params.ScaleApplicationResults, error) {
	v2Args := params.ScaleApplicationsParamsV2{
		Applications: make([]params.ScaleApplicationParamsV2, len(args.Applications)),
	}
	for i, app := range args.Applications {
		v2Args.Applications[i] = params.ScaleApplicationParamsV2{
			ApplicationTag: app.ApplicationTag,
			Scale:          app.Scale,
			ScaleChange:    app.ScaleChange,
			Force:          app.Force,
			// APIv20 does not support attage storage.
			AttachStorage: nil,
		}
	}
	return api.APIv21.ScaleApplications(ctx, v2Args)
}

// GetConstraints returns the constraints for a given application.
func (api *APIBase) GetConstraints(ctx context.Context, args params.Entities) (params.ApplicationGetConstraintsResults, error) {
	if err := api.checkCanRead(ctx); err != nil {
		return params.ApplicationGetConstraintsResults{}, errors.Trace(err)
	}
	results := params.ApplicationGetConstraintsResults{
		Results: make([]params.ApplicationConstraint, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		cons, err := api.getConstraints(ctx, arg.Tag)
		results.Results[i].Constraints = cons
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (api *APIBase) getConstraints(ctx context.Context, entity string) (constraints.Value, error) {
	tag, err := names.ParseTag(entity)
	if err != nil {
		return constraints.Value{}, err
	}
	switch kind := tag.Kind(); kind {
	case names.ApplicationTagKind:
		appID, err := api.applicationService.GetApplicationIDByName(ctx, tag.Id())
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			return constraints.Value{}, errors.NotFoundf("application %s", tag.Id())
		} else if err != nil {
			return constraints.Value{}, errors.Trace(err)
		}
		cons, err := api.applicationService.GetApplicationConstraints(ctx, appID)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			return constraints.Value{}, errors.NotFoundf("application %s", tag.Id())
		} else if err != nil {
			return constraints.Value{}, errors.Trace(err)
		}
		return cons, nil
	default:
		return constraints.Value{}, errors.Errorf("unexpected tag type, expected application, got %s", kind)
	}
}

// SetConstraints sets the constraints for a given application.
func (api *APIBase) SetConstraints(ctx context.Context, args params.SetConstraints) error {
	if err := api.checkCanWrite(ctx); err != nil {
		return err
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}

	appID, err := api.applicationService.GetApplicationIDByName(ctx, args.ApplicationName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return errors.NotFoundf("application %s", args.ApplicationName)
	} else if err != nil {
		return errors.Trace(err)
	}
	err = api.applicationService.SetApplicationConstraints(ctx, appID, args.Constraints)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return errors.NotFoundf("application %s", args.ApplicationName)
	} else if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (api *APIBase) AddRelation(ctx context.Context, args params.AddRelation) (_ params.AddRelationResults, err error) {
	if err := api.checkCanWrite(ctx); err != nil {
		return params.AddRelationResults{}, internalerrors.Capture(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.AddRelationResults{}, internalerrors.Capture(err)
	}

	if len(args.ViaCIDRs) > 0 {
		// Integration via subnets is only for cross model relations.
		return params.AddRelationResults{}, internalerrors.Errorf("cross model relations are disabled until "+
			"backend functionality is moved to domain: %w", errors.NotImplemented)
	}

	if len(args.Endpoints) != 2 {
		return params.AddRelationResults{}, errors.BadRequestf("a relation should have exactly two endpoints")
	}
	ep1, ep2, err := api.relationService.AddRelation(
		ctx, args.Endpoints[0], args.Endpoints[1],
	)
	if err != nil {
		return params.AddRelationResults{}, internalerrors.Errorf(
			"adding relation between endpoints %q and %q: %w",
			args.Endpoints[0], args.Endpoints[1], err,
		)
	}
	return params.AddRelationResults{Endpoints: map[string]params.CharmRelation{
		ep1.ApplicationName: encodeRelation(ep1.Relation),
		ep2.ApplicationName: encodeRelation(ep2.Relation),
	}}, nil
}

// encodeRelation encodes a relation for sending over the wire.
func encodeRelation(rel charm.Relation) params.CharmRelation {
	return params.CharmRelation{
		Name:      rel.Name,
		Role:      string(rel.Role),
		Interface: rel.Interface,
		Optional:  rel.Optional,
		Limit:     rel.Limit,
		Scope:     string(rel.Scope),
	}
}

// DestroyRelation removes the relation between the
// specified endpoints or an id.
func (api *APIBase) DestroyRelation(ctx context.Context, args params.DestroyRelation) (err error) {
	if err := api.checkCanWrite(ctx); err != nil {
		return err
	}
	if err := api.check.RemoveAllowed(ctx); err != nil {
		return internalerrors.Capture(err)
	}

	getUUIDArgs := relation.GetRelationUUIDForRemovalArgs{
		Endpoints:  args.Endpoints,
		RelationID: args.RelationId,
	}
	relUUID, err := api.relationService.GetRelationUUIDForRemoval(ctx, getUUIDArgs)
	if err != nil {
		return internalerrors.Capture(err)
	}

	force := false
	if args.Force != nil {
		force = *args.Force
	}
	var maxWait time.Duration
	if args.MaxWait != nil {
		maxWait = *args.MaxWait
	}

	removalUUID, err := api.removalService.RemoveRelation(ctx, relUUID, force, maxWait)
	if err == nil {
		var msg string
		if len(args.Endpoints) == 2 {
			msg = fmt.Sprintf("%q, %q", args.Endpoints[0], args.Endpoints[1])
		} else {
			msg = fmt.Sprintf("%d", args.RelationId)
		}
		api.logger.Debugf(ctx, "removal uuid %q for relation %q", removalUUID, msg)
	}
	return internalerrors.Capture(err)
}

// SetRelationsSuspended sets the suspended status of the specified relations.
func (api *APIBase) SetRelationsSuspended(ctx context.Context, args params.RelationSuspendedArgs) (params.ErrorResults, error) {
	// Suspending relation is only available for Cross Model Relations
	return params.ErrorResults{}, internalerrors.Errorf("cross model relations are disabled until "+
		"backend functionality is moved to domain: %w", errors.NotImplemented)
}

// Consume adds remote applications to the model without creating any
// relations.
func (api *APIBase) Consume(ctx context.Context, args params.ConsumeApplicationArgsV5) (params.ErrorResults, error) {
	if err := api.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	results := make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		err := api.consumeOne(ctx, arg)
		results[i].Error = apiservererrors.ServerError(err)
	}
	return params.ErrorResults{
		Results: results,
	}, nil
}

func (api *APIBase) consumeOne(ctx context.Context, arg params.ConsumeApplicationArgV5) error {
	sourceModelTag, err := names.ParseModelTag(arg.SourceModelTag)
	if err != nil {
		return internalerrors.Errorf("parsing source model tag: %w", err).Add(coreerrors.BadRequest)
	}

	// Maybe save the details of the controller hosting the offer.
	var offererControllerUUID *string
	if arg.ControllerInfo != nil {
		offererControllerUUID, err = api.saveExternalController(ctx, *arg.ControllerInfo, sourceModelTag)
		if err != nil {
			return internalerrors.Errorf("saving external controller info: %w", err)
		}
	}

	applicationName := arg.ApplicationAlias
	if applicationName == "" {
		applicationName = arg.OfferName
	}

	return api.saveRemoteApplicationOfferer(
		ctx,
		applicationName,
		offererControllerUUID, sourceModelTag.Id(),
		arg.ApplicationOfferDetailsV5,
		arg.Macaroon,
	)
}

func (api *APIBase) saveExternalController(ctx context.Context, info params.ExternalControllerInfo, sourceModelTag names.ModelTag) (*string, error) {
	controllerTag, err := names.ParseControllerTag(info.ControllerTag)
	if err != nil {
		return nil, internalerrors.Errorf("parsing controller tag %q: %w", info.ControllerTag, err).Add(coreerrors.BadRequest)
	}

	// Don't save the controller if it's this controller. It's allowed to
	// consume offers from different models on the same controller.
	if controllerTag.Id() == api.controllerUUID {
		return nil, nil
	}

	// Save the controller details, if we have the information already
	// then update it.
	if err = api.externalControllerService.UpdateExternalController(ctx, crossmodel.ControllerInfo{
		ControllerUUID: controllerTag.Id(),
		Alias:          info.Alias,
		Addrs:          info.Addrs,
		CACert:         info.CACert,
		ModelUUIDs:     []string{sourceModelTag.Id()},
	}); err != nil {
		return nil, internalerrors.Errorf("updating external controller %q: %w", controllerTag.Id(), err)
	}

	return ptr(controllerTag.Id()), nil
}

func (api *APIBase) saveRemoteApplicationOfferer(
	ctx context.Context,
	applicationName string,
	offererControllerUUID *string,
	offererModelUUID string,
	offer params.ApplicationOfferDetailsV5,
	macaroon *macaroon.Macaroon,
) error {

	remoteEps := make([]domaincharm.Relation, len(offer.Endpoints))
	for j, ep := range offer.Endpoints {
		role, err := enocdeRelationRole(ep.Role)
		if err != nil {
			return internalerrors.Errorf("parsing role for endpoint %q: %w", ep.Name, err).Add(coreerrors.BadRequest)
		}

		remoteEps[j] = domaincharm.Relation{
			Name:      ep.Name,
			Role:      role,
			Interface: ep.Interface,
		}
	}

	// TODO (stickupkid): Handle the following case:
	//
	// If a remote application with the same name and endpoints from the same
	// source model already exists, we will use that one. If the status was
	// "terminated", the offer had been removed, so we'll replace the terminated
	// application with a fresh copy.
	//

	return api.crossModelRelationService.AddRemoteApplicationOfferer(ctx, applicationName, crossmodelrelationservice.AddRemoteApplicationOffererArgs{
		OfferUUID:             offer.OfferUUID,
		OffererControllerUUID: offererControllerUUID,
		OffererModelUUID:      offererModelUUID,
		Endpoints:             remoteEps,
		Macaroon:              macaroon,
	})
}

func enocdeRelationRole(role charm.RelationRole) (domaincharm.RelationRole, error) {
	switch role {
	case charm.RoleProvider:
		return domaincharm.RoleProvider, nil
	case charm.RoleRequirer:
		return domaincharm.RoleRequirer, nil
	case charm.RolePeer:
		return "", errors.New("peer relations cannot be cross model")
	default:
		return "", errors.Errorf("unknown role %q", role)
	}
}

// Get returns the charm configuration for an application.
func (api *APIBase) Get(ctx context.Context, args params.ApplicationGet) (params.ApplicationGetResults, error) {
	if err := api.checkCanRead(ctx); err != nil {
		return params.ApplicationGetResults{}, err
	}

	return api.getConfig(ctx, args, describe)
}

// CharmConfig returns charm config for the input list of applications.
func (api *APIBase) CharmConfig(ctx context.Context, args params.ApplicationGetArgs) (params.ApplicationGetConfigResults, error) {
	if err := api.checkCanRead(ctx); err != nil {
		return params.ApplicationGetConfigResults{}, err
	}
	results := params.ApplicationGetConfigResults{
		Results: make([]params.ConfigResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		config, err := api.getMergedAppAndCharmConfig(ctx, arg.ApplicationName)
		results.Results[i].Config = config
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// GetConfig returns the charm config for each of the input applications.
func (api *APIBase) GetConfig(ctx context.Context, args params.Entities) (params.ApplicationGetConfigResults, error) {
	if err := api.checkCanRead(ctx); err != nil {
		return params.ApplicationGetConfigResults{}, err
	}
	results := params.ApplicationGetConfigResults{
		Results: make([]params.ConfigResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if tag.Kind() != names.ApplicationTagKind {
			results.Results[i].Error = apiservererrors.ServerError(
				errors.Errorf("unexpected tag type, expected application, got %s", tag.Kind()))
			continue
		}

		// Always deal with the master branch version of config.
		config, err := api.getMergedAppAndCharmConfig(ctx, tag.Id())
		results.Results[i].Config = config
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// SetConfigs implements the server side of Application.SetConfig.  Both
// application and charm config are set. It does not unset values in
// Config map that are set to an empty string. Unset should be used for that.
func (api *APIBase) SetConfigs(ctx context.Context, args params.ConfigSetArgs) (params.ErrorResults, error) {
	var result params.ErrorResults
	if err := api.checkCanWrite(ctx); err != nil {
		return result, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return result, errors.Trace(err)
	}
	result.Results = make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		result.Results[i] = api.setConfig(ctx, arg)
	}
	return result, nil
}

func (api *APIBase) setConfig(ctx context.Context, arg params.ConfigSet) params.ErrorResult {
	if arg.ConfigYAML != "" {
		return params.ErrorResult{Error: apiservererrors.ServerError(errors.NotImplementedf("config yaml not supported"))}
	}

	appID, err := api.applicationService.GetApplicationIDByName(ctx, arg.ApplicationName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return params.ErrorResult{Error: apiservererrors.ServerError(errors.NotFoundf("application %q", arg.ApplicationName))}
	} else if errors.Is(err, applicationerrors.ApplicationNameNotValid) {
		return params.ErrorResult{Error: apiservererrors.ServerError(errors.NotValidf("application name %q", arg.ApplicationName))}
	} else if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}
	}

	err = api.applicationService.UpdateApplicationConfig(ctx, appID, arg.Config)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return params.ErrorResult{Error: apiservererrors.ServerError(errors.NotFoundf("application %q", arg.ApplicationName))}
	} else if errors.Is(err, applicationerrors.InvalidApplicationConfig) {
		return params.ErrorResult{Error: apiservererrors.ServerError(internalerrors.Errorf("%w%w", err, errors.Hide(errors.NotValid)))}
	} else if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}
	}
	return params.ErrorResult{}
}

// UnsetApplicationsConfig implements the server side of Application.UnsetApplicationsConfig.
func (api *APIBase) UnsetApplicationsConfig(ctx context.Context, args params.ApplicationConfigUnsetArgs) (params.ErrorResults, error) {
	var result params.ErrorResults
	if err := api.checkCanWrite(ctx); err != nil {
		return result, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return result, errors.Trace(err)
	}
	result.Results = make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		err := api.unsetApplicationConfig(ctx, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (api *APIBase) unsetApplicationConfig(ctx context.Context, arg params.ApplicationUnset) error {
	appID, err := api.applicationService.GetApplicationIDByName(ctx, arg.ApplicationName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return errors.NotFoundf("application %s", arg.ApplicationName)
	} else if err != nil {
		return errors.Trace(err)
	}
	err = api.applicationService.UnsetApplicationConfigKeys(ctx, appID, arg.Options)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return errors.NotFoundf("application %s", arg.ApplicationName)
	} else if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// ResolveUnitErrors marks errors on the specified units as resolved.
func (api *APIBase) ResolveUnitErrors(ctx context.Context, p params.UnitsResolved) (params.ErrorResults, error) {
	var result params.ErrorResults
	if err := api.checkCanWrite(ctx); err != nil {
		return result, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return result, errors.Trace(err)
	}

	if p.All && len(p.Tags.Entities) > 0 {
		return params.ErrorResults{}, errors.BadRequestf("cannot resolve all units and specific units")
	}

	resolveMode := resolve.ResolveModeNoHooks
	if p.Retry {
		resolveMode = resolve.ResolveModeRetryHooks
	}

	if p.All {
		err := api.resolveService.ResolveAllUnits(ctx, resolveMode)
		if err != nil {
			return params.ErrorResults{}, errors.Trace(err)
		}
		return params.ErrorResults{}, nil
	}

	result.Results = make([]params.ErrorResult, len(p.Tags.Entities))
	for i, entity := range p.Tags.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = api.resolveService.ResolveUnit(ctx, unitName, resolveMode)
		if errors.Is(err, resolveerrors.UnitNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", unitName))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}

// ApplicationsInfo returns applications information.
//
// TODO (stickupkid/jack-w-shaw): This should be one call to the application
// service. There is no reason to split all these calls into multiple DB calls.
// Once application service is refactored to return the merged config, this
// should be a single call.
func (api *APIBase) ApplicationsInfo(ctx context.Context, in params.Entities) (params.ApplicationInfoResults, error) {
	var result params.ApplicationInfoResults
	if err := api.checkCanRead(ctx); err != nil {
		return result, errors.Trace(err)
	}

	// Get all the space infos before iterating over the application infos.
	allSpaceInfosLookup, err := api.networkService.GetAllSpaces(ctx)
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}

	out := make([]params.ApplicationInfoResult, len(in.Entities))
	for i, one := range in.Entities {
		tag, err := names.ParseApplicationTag(one.Tag)
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		appID, err := api.applicationService.GetApplicationIDByName(ctx, tag.Name)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			out[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", tag.Name)
			continue
		} else if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		appLife, err := api.applicationService.GetApplicationLife(ctx, appID)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			out[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", tag.Name)
			continue
		} else if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		locator, err := api.applicationService.GetCharmLocatorByApplicationName(ctx, tag.Name)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			out[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", tag.Name)
			continue
		} else if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		bindings, err := api.applicationService.GetApplicationEndpointBindings(ctx, tag.Name)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			out[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", tag.Name)
			continue
		} else if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		bindingsMap, err := network.MapBindingsWithSpaceNames(bindings, allSpaceInfosLookup)
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		exposedEndpoints, err := api.applicationService.GetExposedEndpoints(ctx, tag.Name)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			out[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", tag.Name)
			continue
		} else if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		mappedExposedEndpoints, err := api.mapExposedEndpointsFromDomain(ctx, exposedEndpoints)
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		isExposed, err := api.applicationService.IsApplicationExposed(ctx, tag.Name)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			out[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", tag.Name)
			continue
		} else if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		isSubordinate, err := api.applicationService.IsSubordinateApplication(ctx, appID)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			out[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", tag.Name)
			continue
		} else if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		var cons constraints.Value
		if !isSubordinate {
			cons, err = api.applicationService.GetApplicationConstraints(ctx, appID)
			if errors.Is(err, applicationerrors.ApplicationNotFound) {
				out[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", tag.Name)
				continue
			} else if err != nil {
				out[i].Error = apiservererrors.ServerError(err)
				continue
			}
		}

		origin, err := api.applicationService.GetApplicationCharmOrigin(ctx, tag.Name)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			out[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", tag.Name)
			continue
		} else if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// If the applications charm origin is from charm-hub, then build the real
		// channel and send that back.
		var channel string
		if corecharm.CharmHub.Matches(origin.Source.String()) && origin.Channel != nil {
			ch := origin.Channel
			channel = charm.MakePermissiveChannel(ch.Track, string(ch.Risk), ch.Branch).String()
		}

		osType, err := encodeOSType(origin.Platform.OS)
		if err != nil {
			out[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}

		out[i].Result = &params.ApplicationResult{
			Tag:   tag.String(),
			Charm: locator.Name,
			Base: params.Base{
				Name:    osType,
				Channel: origin.Platform.Channel,
			},
			Channel:          channel,
			Constraints:      cons,
			Principal:        !isSubordinate,
			Exposed:          isExposed,
			Life:             string(appLife),
			EndpointBindings: bindingsMap,
			ExposedEndpoints: mappedExposedEndpoints,
		}
	}
	return params.ApplicationInfoResults{
		Results: out,
	}, nil
}

func (api *APIBase) mapExposedEndpointsFromDomain(ctx context.Context, exposedEndpoints map[string]application.ExposedEndpoint) (map[string]params.ExposedEndpoint, error) {
	if len(exposedEndpoints) == 0 {
		return nil, nil
	}

	var (
		err error
		res = make(map[string]params.ExposedEndpoint, len(exposedEndpoints))
	)

	spaceInfos, err := api.networkService.GetAllSpaces(ctx)
	if err != nil {
		return nil, err
	}

	for endpointName, exposeDetails := range exposedEndpoints {
		mappedParam := params.ExposedEndpoint{
			ExposeToCIDRs: exposeDetails.ExposeToCIDRs.Values(),
		}

		if len(exposeDetails.ExposeToSpaceIDs) != 0 {

			spaceNames := make([]string, len(exposeDetails.ExposeToSpaceIDs))
			for i, spaceID := range exposeDetails.ExposeToSpaceIDs.Values() {
				sp := spaceInfos.GetByID(network.SpaceUUID(spaceID))
				if sp == nil {
					return nil, errors.NotFoundf("space with ID %q", spaceID)
				}

				spaceNames[i] = string(sp.Name)
			}
			mappedParam.ExposeToSpaces = spaceNames
		}

		res[endpointName] = mappedParam
	}

	return res, nil
}

// MergeBindings merges operator-defined bindings with the current bindings for
// one or more applications.
func (api *APIBase) MergeBindings(ctx context.Context, in params.ApplicationMergeBindingsArgs) (params.ErrorResults, error) {
	if err := api.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, err
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	res := make([]params.ErrorResult, len(in.Args))
	for i, arg := range in.Args {
		tag, err := names.ParseApplicationTag(arg.ApplicationTag)
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		appID, err := api.applicationService.GetApplicationIDByName(ctx, tag.Id())
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			res[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", tag.Id())
			continue
		} else if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		err = api.applicationService.MergeApplicationEndpointBindings(ctx, appID, transformBindings(arg.Bindings), arg.Force)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			res[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", tag.Id())
			continue
		} else if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return params.ErrorResults{Results: res}, nil
}

// AgentTools is a point of use agent tools requester.
type AgentTools interface {
	AgentTools() (*tools.Tools, error)
}

// AgentVersioner is a point of use agent version object.
type AgentVersioner interface {
	AgentVersion() (semversion.Number, error)
}

var (
	// ErrInvalidAgentVersions is a sentinal error for when we can no longer
	// upgrade juju using 2.5.x agents with 2.6 or greater controllers.
	ErrInvalidAgentVersions = errors.Errorf(
		"Unable to upgrade LXDProfile charms with the current model version. " +
			"Please run juju upgrade-model to upgrade the current model to match your controller.")
)

// UnitsInfo returns unit information for the given entities (units or
// applications).
func (api *APIBase) UnitsInfo(ctx context.Context, in params.Entities) (params.UnitInfoResults, error) {
	if err := api.checkCanRead(ctx); err != nil {
		return params.UnitInfoResults{}, err
	}

	var results []params.UnitInfoResult
	leaders, err := api.leadershipReader.Leaders()
	if err != nil {
		return params.UnitInfoResults{}, errors.Trace(err)
	}
	for _, one := range in.Entities {
		tag, err := names.ParseTag(one.Tag)
		if err != nil {
			results = append(results, params.UnitInfoResult{Error: apiservererrors.ServerError(err)})
			continue
		}

		var unitNames []coreunit.Name
		switch tag.(type) {
		case names.ApplicationTag:
			unitNames, err = api.applicationService.GetUnitNamesForApplication(ctx, tag.Id())
			if errors.Is(err, applicationerrors.ApplicationNotFound) {
				results = append(results, params.UnitInfoResult{Error: apiservererrors.ParamsErrorf(params.CodeNotFound, "application %s not found", tag.Id())})
				continue
			} else if err != nil {
				results = append(results, params.UnitInfoResult{Error: apiservererrors.ServerError(err)})
				continue
			}
		case names.UnitTag:
			unitName, err := coreunit.NewName(tag.Id())
			if err != nil {
				results = append(results, params.UnitInfoResult{Error: apiservererrors.ServerError(err)})
				continue
			}
			unitNames = []coreunit.Name{unitName}
		default:
			results = append(results, params.UnitInfoResult{Error: apiservererrors.ServerError(errors.NotValidf("tag %q", tag))})
		}

		for _, unitName := range unitNames {
			result, err := api.unitResultForUnit(ctx, unitName)
			if err != nil {
				results = append(results, params.UnitInfoResult{Error: apiservererrors.ServerError(err)})
				continue
			}
			if leader := leaders[unitName.Application()]; leader == unitName.String() {
				result.Leader = true
			}
			results = append(results, params.UnitInfoResult{Result: result})
		}
	}
	return params.UnitInfoResults{
		Results: results,
	}, nil
}

// Builds a *params.UnitResult describing the specified unit.
func (api *APIBase) unitResultForUnit(ctx context.Context, unitName coreunit.Name) (*params.UnitResult, error) {
	unitLife, err := api.applicationService.GetUnitLife(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil, errors.NotFoundf("unit %s", unitName)
	} else if err != nil {
		return nil, err
	}

	workloadVersion, err := api.applicationService.GetUnitWorkloadVersion(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil, errors.NotFoundf("unit %s", unitName)
	} else if err != nil {
		return nil, err
	}

	charmLocator, err := api.applicationService.GetCharmLocatorByApplicationName(ctx, unitName.Application())
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil, errors.NotFoundf("application %s", unitName.Application())
	} else if err != nil {
		return nil, err
	}
	curl, err := apiservercharms.CharmURLFromLocator(charmLocator.Name, charmLocator)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	result := &params.UnitResult{
		Tag:             names.NewUnitTag(unitName.String()).String(),
		WorkloadVersion: workloadVersion,
		Charm:           curl,
		Life:            string(unitLife),
	}
	result.RelationData, err = api.relationData(ctx, unitName.Application())
	if err != nil {
		return nil, err
	}

	machineName, err := api.applicationService.GetUnitMachineName(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil, errors.NotFoundf("unit %s", unitName)
	} else if errors.Is(err, applicationerrors.UnitMachineNotAssigned) {
		podInfo, err := api.applicationService.GetUnitK8sPodInfo(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			return nil, errors.NotFoundf("unit %s", unitName)
		} else if err != nil {
			return nil, err
		}
		result.ProviderId = podInfo.ProviderID.String()
		result.Address = podInfo.Address
		result.OpenedPorts = podInfo.Ports

	} else if err != nil {
		return nil, internalerrors.Errorf("getting unit machine name: %w", err)
	} else {
		result.Machine = machineName.String()

		publicAddress, err := api.networkService.GetUnitPublicAddress(ctx, unitName)
		if err == nil {
			result.PublicAddress = publicAddress.Value
		}

		unitUUID, err := api.applicationService.GetUnitUUID(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			return nil, errors.NotFoundf("unit %s", unitName)
		} else if err != nil {
			return nil, err
		}
		// NOTE(achilleasa): this call completely ignores
		// subnets and lumps all port ranges together in a
		// single group. This works fine for pre 2.9 agents
		// as ports where always opened across all subnets.
		openPorts, err := api.openPortsOnUnit(ctx, unitUUID)
		if err != nil {
			return nil, err
		}
		result.OpenedPorts = openPorts
	}
	return result, nil
}

// openPortsOnMachineForUnit returns the unique set of opened ports for the
// specified unit and machine arguments without distinguishing between port
// ranges across subnets. This method is provided for backwards compatibility
// with pre 2.9 agents which assume open-ports apply to all subnets.
func (api *APIBase) openPortsOnUnit(ctx context.Context, unitUUID coreunit.UUID) ([]string, error) {
	var result []string

	groupedPortRanges, err := api.portService.GetUnitOpenedPorts(ctx, unitUUID)
	if err != nil {
		return nil, internalerrors.Errorf("getting opened ports for unit %q: %w", unitUUID, err)
	}
	for _, portRange := range groupedPortRanges.UniquePortRanges() {
		result = append(result, portRange.String())
	}
	return result, nil
}

func (api *APIBase) relationData(ctx context.Context, appName string) ([]params.EndpointRelationData, error) {
	appID, err := api.applicationService.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return nil, internalerrors.Errorf("getting application id for %q: %v", appName, err)
	}
	endpointsData, err := api.relationService.ApplicationRelationsInfo(ctx, appID)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	var result []params.EndpointRelationData
	for _, endpointData := range endpointsData {
		unitRelationData := make(map[string]params.RelationData)
		for k, v := range endpointData.UnitRelationData {
			unitRelationData[k] = params.RelationData{
				InScope:  v.InScope,
				UnitData: v.UnitData,
			}
		}
		result = append(result, params.EndpointRelationData{
			RelationId:       endpointData.RelationID,
			Endpoint:         endpointData.Endpoint,
			CrossModel:       false,
			RelatedEndpoint:  endpointData.RelatedEndpoint,
			ApplicationData:  endpointData.ApplicationData,
			UnitRelationData: unitRelationData,
		})
	}
	return result, nil
}

// Leader returns the unit name of the leader for the given application.
func (api *APIBase) Leader(ctx context.Context, entity params.Entity) (params.StringResult, error) {
	if err := api.checkCanRead(ctx); err != nil {
		return params.StringResult{}, errors.Trace(err)
	}

	result := params.StringResult{}
	application, err := names.ParseApplicationTag(entity.Tag)
	if err != nil {
		return result, err
	}
	leaders, err := api.leadershipReader.Leaders()
	if err != nil {
		return result, errors.Annotate(err, "querying leaders")
	}
	var ok bool
	result.Result, ok = leaders[application.Name]
	if !ok || result.Result == "" {
		result.Error = apiservererrors.ServerError(errors.NotFoundf("leader for %s", entity.Tag))
	}
	return result, nil
}

// DeployFromRepository is a one-stop deployment method for repository
// charms. Only a charm name is required to deploy. If argument validation
// fails, a list of all errors found in validation will be returned. If a
// local resource is provided, details required for uploading the validated
// resource will be returned.
func (api *APIBase) DeployFromRepository(ctx context.Context, args params.DeployFromRepositoryArgs) (params.DeployFromRepositoryResults, error) {
	if err := api.checkCanWrite(ctx); err != nil {
		return params.DeployFromRepositoryResults{}, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.DeployFromRepositoryResults{}, errors.Trace(err)
	}

	results := make([]params.DeployFromRepositoryResult, len(args.Args))
	for i, entity := range args.Args {
		info, pending, errs := api.repoDeploy.DeployFromRepository(ctx, entity)
		if len(errs) > 0 {
			results[i].Errors = apiservererrors.ServerErrors(errs)
			continue
		}
		results[i].Info = info
		results[i].PendingResourceUploads = pending
	}
	return params.DeployFromRepositoryResults{
		Results: results,
	}, nil
}

func (api *APIBase) getOneApplicationStorage(entity params.Entity) (map[string]params.StorageDirectives, error) {
	// TODO(storage): implement and add test.
	return nil, errors.NotImplementedf("GetApplicationStorage")
}

// GetApplicationStorage returns the current storage constraints for the specified applications in bulk.
func (api *APIBase) GetApplicationStorage(ctx context.Context, args params.Entities) (params.ApplicationStorageGetResults, error) {
	resp := params.ApplicationStorageGetResults{
		Results: make([]params.ApplicationStorageGetResult, len(args.Entities)),
	}
	if err := api.checkCanRead(ctx); err != nil {
		return resp, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		sc, err := api.getOneApplicationStorage(entity)
		if err != nil {
			resp.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		resp.Results[i].StorageConstraints = sc
	}
	return resp, nil
}

func (api *APIBase) updateOneApplicationStorage(storageUpdate params.ApplicationStorageUpdate) error {
	// TODO(storage): implement and add test.
	return errors.NotImplementedf("UpdateApplicationStorage")
}

// UpdateApplicationStorage updates the storage constraints for multiple existing applications in bulk.
// We do not create new storage constraints since it is handled by addDefaultStorageConstraints during
// application deployment. The storage constraints passed are validated against the charm's declared storage meta.
// The following apiserver codes can be returned in each ErrorResult:
//   - [params.CodeNotSupported]: If the update request includes a storage name not supported by the charm.
func (api *APIBase) UpdateApplicationStorage(ctx context.Context, args params.ApplicationStorageUpdateRequest) (params.ErrorResults, error) {
	resp := params.ErrorResults{}
	if err := api.checkCanWrite(ctx); err != nil {
		return resp, errors.Trace(err)
	}

	res := make([]params.ErrorResult, len(args.ApplicationStorageUpdates))
	resp.Results = res

	for i, storageUpdate := range args.ApplicationStorageUpdates {
		err := api.updateOneApplicationStorage(storageUpdate)
		res[i].Error = apiservererrors.ServerError(err)
	}

	return resp, nil
}
