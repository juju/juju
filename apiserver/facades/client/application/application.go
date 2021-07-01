// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package application contains api calls for functionality
// related to deploying and managing applications and their
// related charms.
package application

import (
	"math"
	"net"
	"reflect"

	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/schema"
	"github.com/juju/version/v2"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/macaroon.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes/provider"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.apiserver.application")

// APIv4 provides the Application API facade for versions 1-4.
type APIv4 struct {
	*APIv5
}

// APIv5 provides the Application API facade for version 5.
type APIv5 struct {
	*APIv6
}

// APIv6 provides the Application API facade for version 6.
type APIv6 struct {
	*APIv7
}

// APIv7 provides the Application API facade for version 7.
type APIv7 struct {
	*APIv8
}

// APIv8 provides the Application API facade for version 8.
type APIv8 struct {
	*APIv9
}

// APIv9 provides the Application API facade for version 9.
type APIv9 struct {
	*APIv10
}

// APIv10 provides the Application API facade for version 10.
// It adds --force and --max-wait parameters to remove-saas.
type APIv10 struct {
	*APIv11
}

// APIv11 provides the Application API facade for version 11.
// The Get call also returns the current endpoint bindings while the SetCharm
// call access a map of operator-defined bindings.
type APIv11 struct {
	*APIv12
}

// APIv12 provides the Application API facade for version 12.
// It adds the UnitsInfo method.
type APIv12 struct {
	*APIv13
}

// APIv13 provides the Application API facade for version 13.
// It adds CharmOrigin. The ApplicationsInfo call populates the exposed
// endpoints field in its response entries.
type APIv13 struct {
	*APIBase
}

// APIBase implements the shared application interface and is the concrete
// implementation of the api end point.
//
// API provides the Application API facade for version 5.
type APIBase struct {
	backend       Backend
	storageAccess storageInterface

	authorizer   facade.Authorizer
	check        BlockChecker
	updateSeries UpdateSeries

	model     Model
	modelType state.ModelType

	resources        facade.Resources
	leadershipReader leadership.Reader

	// TODO(axw) stateCharm only exists because I ran out
	// of time unwinding all of the tendrils of state. We
	// should pass a charm.Charm and charm.URL back into
	// state wherever we pass in a state.Charm currently.
	stateCharm func(Charm) *state.Charm

	storagePoolManager    poolmanager.PoolManager
	registry              storage.ProviderRegistry
	caasBroker            caasBrokerInterface
	deployApplicationFunc func(ApplicationDeployer, DeployApplicationParams) (Application, error)
}

// NewFacadeV4 provides the signature required for facade registration
// for versions 1-4.
func NewFacadeV4(ctx facade.Context) (*APIv4, error) {
	api, err := NewFacadeV5(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv4{api}, nil
}

// NewFacadeV5 provides the signature required for facade registration
// for version 5.
func NewFacadeV5(ctx facade.Context) (*APIv5, error) {
	api, err := NewFacadeV6(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv5{api}, nil
}

// NewFacadeV6 provides the signature required for facade registration
// for version 6.
func NewFacadeV6(ctx facade.Context) (*APIv6, error) {
	api, err := NewFacadeV7(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv6{api}, nil
}

// NewFacadeV7 provides the signature required for facade registration
// for version 7.
func NewFacadeV7(ctx facade.Context) (*APIv7, error) {
	api, err := NewFacadeV8(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv7{api}, nil
}

// NewFacadeV8 provides the signature required for facade registration
// for version 8.
func NewFacadeV8(ctx facade.Context) (*APIv8, error) {
	api, err := NewFacadeV9(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv8{api}, nil
}

func NewFacadeV9(ctx facade.Context) (*APIv9, error) {
	api, err := NewFacadeV10(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv9{api}, nil
}

func NewFacadeV10(ctx facade.Context) (*APIv10, error) {
	api, err := NewFacadeV11(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv10{api}, nil
}

func NewFacadeV11(ctx facade.Context) (*APIv11, error) {
	api, err := NewFacadeV12(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv11{api}, nil
}

func NewFacadeV12(ctx facade.Context) (*APIv12, error) {
	api, err := NewFacadeV13(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv12{api}, nil
}

func NewFacadeV13(ctx facade.Context) (*APIv13, error) {
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv13{api}, nil
}

type caasBrokerInterface interface {
	ValidateStorageClass(config map[string]interface{}) error
	Version() (*version.Number, error)
}

func newFacadeBase(ctx facade.Context) (*APIBase, error) {
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Annotate(err, "getting model")
	}
	storageAccess, err := getStorageState(ctx.State())
	if err != nil {
		return nil, errors.Annotate(err, "getting state")
	}
	blockChecker := common.NewBlockChecker(ctx.State())
	stateCharm := CharmToStateCharm

	var (
		storagePoolManager poolmanager.PoolManager
		registry           storage.ProviderRegistry
		caasBroker         caas.Broker
	)
	if model.Type() == state.ModelTypeCAAS {
		caasBroker, err = stateenvirons.GetNewCAASBrokerFunc(caas.New)(model)
		if err != nil {
			return nil, errors.Annotate(err, "getting caas client")
		}
		registry = stateenvirons.NewStorageProviderRegistry(caasBroker)
		storagePoolManager = poolmanager.New(state.NewStateSettings(ctx.State()), registry)
	}

	resources := ctx.Resources()

	leadershipReader, err := ctx.LeadershipReader(ctx.State().ModelUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	state := &stateShim{ctx.State()}

	modelCfg, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}

	clientLogger := logger.Child("client")
	options := []charmhub.Option{
		// TODO (stickupkid): Get the http transport from the facade context
		charmhub.WithHTTPTransport(charmhub.DefaultHTTPTransport),
	}

	var chCfg charmhub.Config
	chURL, ok := modelCfg.CharmHubURL()
	if ok {
		chCfg, err = charmhub.CharmHubConfigFromURL(chURL, clientLogger, options...)
	} else {
		chCfg, err = charmhub.CharmHubConfig(clientLogger, options...)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	chClient, err := charmhub.NewClient(chCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	updateSeries := NewUpdateSeriesAPI(state, makeUpdateSeriesValidator(chClient))

	return NewAPIBase(
		state,
		storageAccess,
		ctx.Auth(),
		updateSeries,
		blockChecker,
		&modelShim{Model: model}, // modelShim wraps the AllPorts() API.
		leadershipReader,
		stateCharm,
		DeployApplication,
		storagePoolManager,
		registry,
		resources,
		caasBroker,
	)
}

// NewAPIBase returns a new application API facade.
func NewAPIBase(
	backend Backend,
	storageAccess storageInterface,
	authorizer facade.Authorizer,
	updateSeries UpdateSeries,
	blockChecker BlockChecker,
	model Model,
	leadershipReader leadership.Reader,
	stateCharm func(Charm) *state.Charm,
	deployApplication func(ApplicationDeployer, DeployApplicationParams) (Application, error),
	storagePoolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
	resources facade.Resources,
	caasBroker caasBrokerInterface,
) (*APIBase, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	return &APIBase{
		backend:               backend,
		storageAccess:         storageAccess,
		authorizer:            authorizer,
		updateSeries:          updateSeries,
		check:                 blockChecker,
		model:                 model,
		modelType:             model.Type(),
		leadershipReader:      leadershipReader,
		stateCharm:            stateCharm,
		deployApplicationFunc: deployApplication,
		storagePoolManager:    storagePoolManager,
		registry:              registry,
		resources:             resources,
		caasBroker:            caasBroker,
	}, nil
}

func (api *APIBase) checkPermission(tag names.Tag, perm permission.Access) error {
	allowed, err := api.authorizer.HasPermission(perm, tag)
	if err != nil {
		return errors.Trace(err)
	}
	if !allowed {
		return apiservererrors.ErrPerm
	}
	return nil
}

func (api *APIBase) checkCanRead() error {
	return api.checkPermission(api.model.ModelTag(), permission.ReadAccess)
}

func (api *APIBase) checkCanWrite() error {
	return api.checkPermission(api.model.ModelTag(), permission.WriteAccess)
}

// SetMetricCredentials sets credentials on the application.
func (api *APIBase) SetMetricCredentials(args params.ApplicationMetricCredentials) (params.ErrorResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Creds)),
	}
	if len(args.Creds) == 0 {
		return result, nil
	}
	for i, a := range args.Creds {
		oneApplication, err := api.backend.Application(a.ApplicationName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = oneApplication.SetMetricCredentials(a.MetricCredentials)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

// Deploy fetches the charms from the charm store and deploys them
// using the specified placement directives.
// V5 deploy did not support policy, so pass through an empty string.
func (api *APIv5) Deploy(args params.ApplicationsDeployV5) (params.ErrorResults, error) {
	noDefinedPolicy := ""
	var newArgs params.ApplicationsDeploy
	for _, value := range args.Applications {
		newArgs.Applications = append(newArgs.Applications, params.ApplicationDeploy{
			ApplicationName:  value.ApplicationName,
			Series:           value.Series,
			CharmURL:         value.CharmURL,
			Channel:          value.Channel,
			NumUnits:         value.NumUnits,
			Config:           value.Config,
			ConfigYAML:       value.ConfigYAML,
			Constraints:      value.Constraints,
			Placement:        value.Placement,
			Policy:           noDefinedPolicy,
			Storage:          value.Storage,
			AttachStorage:    value.AttachStorage,
			EndpointBindings: value.EndpointBindings,
			Resources:        value.Resources,
		})
	}
	return api.APIBase.Deploy(newArgs)
}

// Deploy fetches the charms from the charm store and deploys them
// using the specified placement directives.
// V6 deploy did not support devices, so pass through an empty map.
func (api *APIv6) Deploy(args params.ApplicationsDeployV6) (params.ErrorResults, error) {
	var newArgs params.ApplicationsDeploy
	for _, value := range args.Applications {
		newArgs.Applications = append(newArgs.Applications, params.ApplicationDeploy{
			ApplicationName:  value.ApplicationName,
			Series:           value.Series,
			CharmURL:         value.CharmURL,
			Channel:          value.Channel,
			NumUnits:         value.NumUnits,
			Config:           value.Config,
			ConfigYAML:       value.ConfigYAML,
			Constraints:      value.Constraints,
			Placement:        value.Placement,
			Policy:           value.Policy,
			Devices:          nil, // set Devices to nil because v6 and lower versions do not support it
			Storage:          value.Storage,
			AttachStorage:    value.AttachStorage,
			EndpointBindings: value.EndpointBindings,
			Resources:        value.Resources,
		})
	}
	return api.APIBase.Deploy(newArgs)
}

// Deploy fetches the charms from the charm store and deploys them
// using the specified placement directives.
// V12 deploy did not CharmOrigin, so pass through an unknown source.
func (api *APIv12) Deploy(args params.ApplicationsDeployV12) (params.ErrorResults, error) {
	var newArgs params.ApplicationsDeploy
	for _, value := range args.Applications {
		newArgs.Applications = append(newArgs.Applications, params.ApplicationDeploy{
			ApplicationName:  value.ApplicationName,
			Series:           value.Series,
			CharmURL:         value.CharmURL,
			Channel:          value.Channel,
			NumUnits:         value.NumUnits,
			Config:           value.Config,
			ConfigYAML:       value.ConfigYAML,
			Constraints:      value.Constraints,
			Placement:        value.Placement,
			Policy:           value.Policy,
			Devices:          value.Devices,
			Storage:          value.Storage,
			AttachStorage:    value.AttachStorage,
			EndpointBindings: value.EndpointBindings,
			Resources:        value.Resources,
		})
	}
	return api.APIBase.Deploy(newArgs)
}

// Deploy fetches the charms from the charm store and deploys them
// using the specified placement directives.
func (api *APIBase) Deploy(args params.ApplicationsDeploy) (params.ErrorResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Applications)),
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return result, errors.Trace(err)
	}

	for i, arg := range args.Applications {
		err := deployApplication(
			api.backend,
			api.model,
			api.stateCharm,
			arg,
			api.deployApplicationFunc,
			api.storagePoolManager,
			api.registry,
			api.caasBroker,
		)
		result.Results[i].Error = apiservererrors.ServerError(err)

		if err != nil && len(arg.Resources) != 0 {
			// Remove any pending resources - these would have been
			// converted into real resources if the application had
			// been created successfully, but will otherwise be
			// leaked. lp:1705730
			// TODO(babbageclunk): rework the deploy API so the
			// resources are created transactionally to avoid needing
			// to do this.
			resources, err := api.backend.Resources()
			if err != nil {
				logger.Errorf("couldn't get backend.Resources")
				continue
			}
			err = resources.RemovePendingAppResources(arg.ApplicationName, arg.Resources)
			if err != nil {
				logger.Errorf("couldn't remove pending resources for %q", arg.ApplicationName)
			}
		}
	}
	return result, nil
}

func applicationConfigSchema(modelType state.ModelType) (environschema.Fields, schema.Defaults, error) {
	if modelType != state.ModelTypeCAAS {
		return trustFields, trustDefaults, nil
	}
	// TODO(caas) - get the schema from the provider
	defaults := caas.ConfigDefaults(k8s.ConfigDefaults())
	configSchema, err := caas.ConfigSchema(k8s.ConfigSchema())
	if err != nil {
		return nil, nil, err
	}
	return AddTrustSchemaAndDefaults(configSchema, defaults)
}

func splitApplicationAndCharmConfig(modelType state.ModelType, inConfig map[string]string) (
	appCfg map[string]interface{},
	charmCfg map[string]string,
	_ error,
) {

	providerSchema, _, err := applicationConfigSchema(modelType)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	appConfigKeys := application.KnownConfigKeys(providerSchema)

	appConfigAttrs := make(map[string]interface{})
	charmConfig := make(map[string]string)
	for k, v := range inConfig {
		if appConfigKeys.Contains(k) {
			appConfigAttrs[k] = v
		} else {
			charmConfig[k] = v
		}
	}
	return appConfigAttrs, charmConfig, nil
}

// splitApplicationAndCharmConfigFromYAML extracts app specific settings from a charm config YAML
// and returns those app settings plus a YAML with just the charm settings left behind.
func splitApplicationAndCharmConfigFromYAML(modelType state.ModelType, inYaml, appName string) (
	appCfg map[string]interface{},
	outYaml string,
	_ error,
) {
	var allSettings map[string]interface{}
	if err := goyaml.Unmarshal([]byte(inYaml), &allSettings); err != nil {
		return nil, "", errors.Annotate(err, "cannot parse settings data")
	}
	settings, ok := allSettings[appName].(map[interface{}]interface{})
	if !ok {
		// Application key not present; it might be 'juju get' output.
		if _, err := charmConfigFromConfigValues(allSettings); err != nil {
			return nil, "", errors.Errorf("no settings found for %q", appName)
		}

		return nil, inYaml, nil
	}

	providerSchema, _, err := applicationConfigSchema(modelType)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	appConfigKeys := application.KnownConfigKeys(providerSchema)

	appConfigAttrs := make(map[string]interface{})
	for k, v := range settings {
		if key, ok := k.(string); ok && appConfigKeys.Contains(key) {
			appConfigAttrs[key] = v
			delete(settings, k)
		}
	}
	if len(settings) == 0 {
		return appConfigAttrs, "", nil
	}

	allSettings[appName] = settings
	charmConfig, err := goyaml.Marshal(allSettings)
	if err != nil {
		return nil, "", errors.Annotate(err, "cannot marshall charm settings")
	}
	return appConfigAttrs, string(charmConfig), nil
}

func caasPrecheck(
	ch Charm,
	controllerCfg controller.Config,
	model Model,
	args params.ApplicationDeploy,
	storagePoolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
	caasBroker caasBrokerInterface,
) error {
	if len(args.AttachStorage) > 0 {
		return errors.Errorf(
			"AttachStorage may not be specified for container models",
		)
	}
	if len(args.Placement) > 1 {
		return errors.Errorf(
			"only 1 placement directive is supported for container models, got %d",
			len(args.Placement),
		)
	}
	for _, s := range ch.Meta().Storage {
		if s.Type == charm.StorageBlock {
			return errors.Errorf("block storage %q is not supported for container charms", s.Name)
		}
	}
	serviceType := args.Config[k8s.ServiceTypeConfigKey]
	if _, err := k8s.CaasServiceToK8s(caas.ServiceType(serviceType)); err != nil {
		return errors.NotValidf("service type %q", serviceType)
	}

	cfg, err := model.ModelConfig()
	if err != nil {
		return errors.Trace(err)
	}

	// For older charms, operator-storage model config is mandatory.
	if k8s.RequireOperatorStorage(ch) {
		storageClassName, _ := cfg.AllAttrs()[k8sconstants.OperatorStorageKey].(string)
		if storageClassName == "" {
			return errors.New(
				"deploying this Kubernetes application requires a suitable storage class.\n" +
					"None have been configured. Set the operator-storage model config to " +
					"specify which storage class should be used to allocate operator storage.\n" +
					"See https://discourse.jujucharms.com/t/getting-started/152.",
			)
		}
		sp, err := caasoperatorprovisioner.CharmStorageParams("", storageClassName, cfg, "", storagePoolManager, registry)
		if err != nil {
			return errors.Annotatef(err, "getting operator storage params for %q", args.ApplicationName)
		}
		if sp.Provider != string(k8sconstants.StorageProviderType) {
			poolName := cfg.AllAttrs()[k8sconstants.OperatorStorageKey]
			return errors.Errorf(
				"the %q storage pool requires a provider type of %q, not %q", poolName, k8sconstants.StorageProviderType, sp.Provider)
		}
		if err := caasBroker.ValidateStorageClass(sp.Attributes); err != nil {
			return errors.Trace(err)
		}
	}

	workloadStorageClass, _ := cfg.AllAttrs()[k8sconstants.WorkloadStorageKey].(string)
	for storageName, cons := range args.Storage {
		if cons.Pool == "" && workloadStorageClass == "" {
			return errors.Errorf("storage pool for %q must be specified since there's no model default storage class", storageName)
		}
		_, err := caasoperatorprovisioner.CharmStorageParams("", workloadStorageClass, cfg, cons.Pool, storagePoolManager, registry)
		if err != nil {
			return errors.Annotatef(err, "getting workload storage params for %q", args.ApplicationName)
		}
	}

	caasVersion, err := caasBroker.Version()
	if err != nil {
		return errors.Trace(err)
	}
	if err := checkCAASMinVersion(ch, caasVersion); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// deployApplication fetches the charm from the charm store and deploys it.
// The logic has been factored out into a common function which is called by
// both the legacy API on the client facade, as well as the new application facade.
func deployApplication(
	backend Backend,
	model Model,
	stateCharm func(Charm) *state.Charm,
	args params.ApplicationDeploy,
	deployApplicationFunc func(ApplicationDeployer, DeployApplicationParams) (Application, error),
	storagePoolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
	caasBroker caasBrokerInterface,
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
	if err := checkMachinePlacement(backend, args); err != nil {
		return errors.Trace(err)
	}

	// Try to find the charm URL in state first.
	ch, err := backend.Charm(curl)
	if err != nil {
		return errors.Trace(err)
	}

	minver := ch.Meta().MinJujuVersion
	if err := jujuversion.CheckJujuMinVersion(minver, jujuversion.Current); err != nil {
		return errors.Trace(err)
	}

	modelType := model.Type()
	if modelType != state.ModelTypeIAAS {
		cfg, err := backend.ControllerConfig()
		if err != nil {
			return errors.Trace(err)
		}
		if err := caasPrecheck(ch, cfg, model, args, storagePoolManager, registry, caasBroker); err != nil {
			return errors.Trace(err)
		}
	}

	appConfig, _, charmSettings, err := parseCharmSettings(modelType, ch, args.ApplicationName, args.Config, args.ConfigYAML)
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

	bindings, err := state.NewBindings(backend, args.EndpointBindings)
	if err != nil {
		return errors.Trace(err)
	}
	origin, err := convertCharmOrigin(args.CharmOrigin, curl, args.Channel)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = deployApplicationFunc(backend, DeployApplicationParams{
		ApplicationName:   args.ApplicationName,
		Series:            args.Series,
		Charm:             stateCharm(ch),
		CharmOrigin:       origin,
		Channel:           csparams.Channel(args.Channel),
		NumUnits:          args.NumUnits,
		ApplicationConfig: appConfig,
		CharmConfig:       charmSettings,
		Constraints:       args.Constraints,
		Placement:         args.Placement,
		Storage:           args.Storage,
		Devices:           args.Devices,
		AttachStorage:     attachStorage,
		EndpointBindings:  bindings.Map(),
		Resources:         args.Resources,
	})
	return errors.Trace(err)
}

func convertCharmOrigin(origin *params.CharmOrigin, curl *charm.URL, charmStoreChannel string) (corecharm.Origin, error) {
	var (
		originType string
		platform   corecharm.Platform
	)
	if origin != nil {
		originType = origin.Type
		platform = corecharm.Platform{
			Architecture: origin.Architecture,
			OS:           origin.OS,
			Series:       origin.Series,
		}
	}

	switch {
	case origin == nil || origin.Source == "" || origin.Source == "charm-store":
		var rev *int
		if curl.Revision != -1 {
			rev = &curl.Revision
		}
		var ch *charm.Channel
		if charmStoreChannel != "" {
			ch = &charm.Channel{
				Risk: charm.Risk(charmStoreChannel),
			}
		}
		return corecharm.Origin{
			Type:     originType,
			Source:   corecharm.CharmStore,
			Revision: rev,
			Channel:  ch,
			Platform: platform,
		}, nil
	case origin.Source == "local":
		return corecharm.Origin{
			Type:     originType,
			Source:   corecharm.Local,
			Revision: &curl.Revision,
			Platform: platform,
		}, nil
	}

	var track string
	if origin.Track != nil {
		track = *origin.Track
	}
	// We do guarantee that there will be a risk value.
	// Ignore the error, as only caused by risk as an
	// empty string.
	var channel *charm.Channel
	if ch, err := charm.MakeChannel(track, origin.Risk, ""); err == nil {
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

// parseCharmSettings parses, verifies and combines the config settings for a
// charm as specified by the provided config map and config yaml payload. Any
// model-specific application settings will be automatically extracted and
// returned back as an *application.Config.
func parseCharmSettings(modelType state.ModelType, ch Charm, appName string, config map[string]string, configYaml string) (*application.Config, environschema.Fields, charm.Settings, error) {
	// Split out the app config from the charm config for any config
	// passed in as a map as opposed to YAML.
	var (
		applicationConfig map[string]interface{}
		charmConfig       map[string]string
		err               error
	)
	if len(config) > 0 {
		if applicationConfig, charmConfig, err = splitApplicationAndCharmConfig(modelType, config); err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
	}

	// Split out the app config from the charm config for any config
	// passed in as YAML.
	var (
		charmYamlConfig string
		appSettings     = make(map[string]interface{})
	)
	if len(configYaml) != 0 {
		if appSettings, charmYamlConfig, err = splitApplicationAndCharmConfigFromYAML(modelType, configYaml, appName); err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
	}

	// Entries from the string-based config map always override any entries
	// provided via the YAML payload.
	for k, v := range applicationConfig {
		appSettings[k] = v
	}

	appCfgSchema, defaults, err := applicationConfigSchema(modelType)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	appConfig, err := application.NewConfig(appSettings, appCfgSchema, defaults)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	charmSettings := make(charm.Settings)

	// If there isn't a charm YAML, then we can just return the charmConfig as
	// the settings and no need to attempt to parse an empty yaml.
	if len(charmYamlConfig) == 0 {
		for k, v := range charmConfig {
			charmSettings[k] = v
		}
		return appConfig, appCfgSchema, charmSettings, nil
	}

	// Parse the charm YAML and check the yaml against the charm config.
	if charmSettings, err = ch.Config().ParseSettingsYAML([]byte(charmYamlConfig), appName); err != nil {
		// Check if this is 'juju get' output and parse it as such
		jujuGetSettings, pErr := charmConfigFromYamlConfigValues(charmYamlConfig)
		if pErr != nil {
			// Not 'juju output' either; return original error
			return nil, nil, nil, errors.Trace(err)
		}
		charmSettings = jujuGetSettings
	}

	// Entries from the string-based config map always override any entries
	// provided via the YAML payload.
	if len(charmConfig) != 0 {
		// Parse config in a compatible way (see function comment).
		overrideSettings, err := parseSettingsCompatible(ch.Config(), charmConfig)
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
		for k, v := range overrideSettings {
			charmSettings[k] = v
		}
	}

	return appConfig, appCfgSchema, charmSettings, nil
}

// checkMachinePlacement does a non-exhaustive validation of any supplied
// placement directives.
// If the placement scope is for a machine, ensure that the machine exists.
// If the placement is for a machine or a container on an existing machine,
// check that the machine is not locked for series upgrade.
func checkMachinePlacement(backend Backend, args params.ApplicationDeploy) error {
	errTemplate := "cannot deploy %q to machine %s"
	app := args.ApplicationName

	for _, p := range args.Placement {
		dir := p.Directive

		toProvisionedMachine := p.Scope == instance.MachineScope
		if !toProvisionedMachine && dir == "" {
			continue
		}

		m, err := backend.Machine(dir)
		if err != nil {
			if errors.IsNotFound(err) && !toProvisionedMachine {
				continue
			}
			return errors.Annotatef(err, errTemplate, app, dir)
		}

		locked, err := m.IsLockedForSeriesUpgrade()
		if locked {
			err = errors.New("machine is locked for series upgrade")
		}
		if err != nil {
			return errors.Annotatef(err, errTemplate, app, dir)
		}

		locked, err = m.IsParentLockedForSeriesUpgrade()
		if locked {
			err = errors.New("parent machine is locked for series upgrade")
		}
		if err != nil {
			return errors.Annotatef(err, errTemplate, app, dir)
		}
	}

	return nil
}

// parseSettingsCompatible parses setting strings in a way that is
// compatible with the behavior before this CL based on the issue
// http://pad.lv/1194945. Until then setting an option to an empty
// string caused it to reset to the default value. We now allow
// empty strings as actual values, but we want to preserve the API
// behavior.
func parseSettingsCompatible(charmConfig *charm.Config, settings map[string]string) (charm.Settings, error) {
	setSettings := map[string]string{}
	unsetSettings := charm.Settings{}
	// Split settings into those which set and those which unset a value.
	for name, value := range settings {
		if value == "" {
			unsetSettings[name] = nil
			continue
		}
		setSettings[name] = value
	}
	// Validate the settings.
	changes, err := charmConfig.ParseSettingsStrings(setSettings)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Validate the unsettings and merge them into the changes.
	unsetSettings, err = charmConfig.ValidateSettings(unsetSettings)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for name := range unsetSettings {
		changes[name] = nil
	}
	return changes, nil
}

type setCharmParams struct {
	AppName               string
	Application           Application
	CharmOrigin           *params.CharmOrigin
	Channel               csparams.Channel
	ConfigSettingsStrings map[string]string
	ConfigSettingsYAML    string
	ResourceIDs           map[string]string
	StorageConstraints    map[string]params.StorageConstraints
	EndpointBindings      map[string]string
	Force                 forceParams
}

type forceParams struct {
	ForceSeries, ForceUnits, Force bool
}

// Update updates the application attributes, including charm URL,
// minimum number of units, charm config and constraints.
// All parameters in params.ApplicationUpdate except the application name are optional.
// Note: Updating the charm-url via Update is no longer supported.  See SetCharm.
// Note: This method is no longer supported with facade v13.  See: SetCharm, SetConfigs.
func (api *APIv12) Update(args params.ApplicationUpdate) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if !args.ForceCharmURL {
		if err := api.check.ChangeAllowed(); err != nil {
			return errors.Trace(err)
		}
	}
	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}

	// Set the charm for the given application.
	if args.CharmURL != "" {
		return errors.NotSupportedf("updating charm url, see SetCharm")
	}
	// Set the minimum number of units for the given application.
	if args.MinUnits != nil {
		if err = app.SetMinUnits(*args.MinUnits); err != nil {
			return errors.Trace(err)
		}
	}

	if err := api.setConfig(app, args.Generation, args.SettingsYAML, args.SettingsStrings); err != nil {
		return errors.Trace(err)
	}

	// Update application's constraints.
	if args.Constraints != nil {
		return app.SetConstraints(*args.Constraints)
	}
	return nil
}

func (api *APIBase) setConfig(app Application, generation, settingsYAML string, settingsStrings map[string]string) error {
	// We need a guard on the API server-side for direct API callers such as
	// python-libjuju, and for older clients.
	// Always default to the master branch.
	if generation == "" {
		generation = model.GenerationMaster
	}

	// Update settings for charm and/or application.
	ch, _, err := app.Charm()
	if err != nil {
		return errors.Annotate(err, "obtaining charm for this application")
	}

	appConfig, appConfigSchema, charmSettings, err := parseCharmSettings(api.modelType, ch, app.Name(), settingsStrings, settingsYAML)
	if err != nil {
		return errors.Annotate(err, "parsing settings for application")
	}

	var configChanged bool
	if len(charmSettings) != 0 {
		if err = app.UpdateCharmConfig(generation, charmSettings); err != nil {
			return errors.Annotate(err, "updating charm config settings")
		}
		configChanged = true
	}
	if cfgAttrs := appConfig.Attributes(); len(cfgAttrs) > 0 {
		if err = app.UpdateApplicationConfig(cfgAttrs, nil, appConfigSchema, nil); err != nil {
			return errors.Annotate(err, "updating application config settings")
		}
		configChanged = true
	}

	// If the config change is generational, add the app to the generation.
	if configChanged && generation != model.GenerationMaster {
		if err := api.addAppToBranch(generation, app.Name()); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// UpdateApplicationSeries updates the application series. Series for
// subordinates updated too.
func (api *APIBase) UpdateApplicationSeries(args params.UpdateSeriesArgs) (params.ErrorResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.ErrorResults{}, err
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := api.updateOneApplicationSeries(arg)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (api *APIBase) updateOneApplicationSeries(arg params.UpdateSeriesArg) error {
	return api.updateSeries.UpdateSeries(arg.Entity.Tag, arg.Series, arg.Force)
}

// SetCharm sets the charm for a given for the application.
func (api *APIv12) SetCharm(args params.ApplicationSetCharmV12) error {
	newArgs := params.ApplicationSetCharm{
		ApplicationName:    args.ApplicationName,
		Generation:         args.Generation,
		CharmURL:           args.CharmURL,
		Channel:            args.Channel,
		ConfigSettings:     args.ConfigSettings,
		ConfigSettingsYAML: args.ConfigSettingsYAML,
		Force:              args.Force,
		ForceUnits:         args.ForceUnits,
		ForceSeries:        args.ForceSeries,
		ResourceIDs:        args.ResourceIDs,
		StorageConstraints: args.StorageConstraints,
		EndpointBindings:   args.EndpointBindings,
	}
	return api.APIBase.SetCharm(newArgs)
}

// SetCharm sets the charm for a given for the application.
func (api *APIBase) SetCharm(args params.ApplicationSetCharm) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	// when forced units in error, don't block
	if !args.ForceUnits {
		if err := api.check.ChangeAllowed(); err != nil {
			return errors.Trace(err)
		}
	}
	oneApplication, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	channel := csparams.Channel(args.Channel)
	return api.setCharmWithAgentValidation(
		setCharmParams{
			AppName:               args.ApplicationName,
			Application:           oneApplication,
			CharmOrigin:           args.CharmOrigin,
			Channel:               channel,
			ConfigSettingsStrings: args.ConfigSettings,
			ConfigSettingsYAML:    args.ConfigSettingsYAML,
			ResourceIDs:           args.ResourceIDs,
			StorageConstraints:    args.StorageConstraints,
			EndpointBindings:      args.EndpointBindings,
			Force: forceParams{
				ForceSeries: args.ForceSeries,
				ForceUnits:  args.ForceUnits,
				Force:       args.Force,
			},
		},
		args.CharmURL,
	)
}

var (
	deploymentInfoUpgradeMessage = `
Juju on containers does not support updating deployment info for services.
The new charm's metadata contains updated deployment info.
You'll need to deploy a new charm rather than upgrading if you need this change.
`[1:]

	storageUpgradeMessage = `
Juju on containers does not support updating storage on a statefulset.
The new charm's metadata contains updated storage declarations.
You'll need to deploy a new charm rather than upgrading if you need this change.
`[1:]

	devicesUpgradeMessage = `
Juju on containers does not support updating node selectors (configured from charm devices).
The new charm's metadata contains updated device declarations.
You'll need to deploy a new charm rather than upgrading if you need this change.
`[1:]
)

// setCharmWithAgentValidation checks the agent versions of the application
// and unit before continuing on. These checks are important to prevent old
// code running at the same time as the new code. If you encounter the error,
// the correct and only work around is to upgrade the units to match the
// controller.
func (api *APIBase) setCharmWithAgentValidation(
	params setCharmParams,
	url string,
) error {
	curl, err := charm.ParseURL(url)
	if err != nil {
		return errors.Trace(err)
	}
	newCharm, err := api.backend.Charm(curl)
	if err != nil {
		return errors.Trace(err)
	}
	oneApplication := params.Application
	currentCharm, _, err := oneApplication.Charm()
	if err != nil {
		logger.Debugf("Unable to locate current charm: %v", err)
	}
	newOrigin, err := convertCharmOrigin(params.CharmOrigin, curl, string(params.Channel))
	if err != nil {
		return errors.Trace(err)
	}
	if api.modelType == state.ModelTypeCAAS {
		// We need to disallow updates that k8s does not yet support,
		// eg changing the filesystem or device directives, or deployment info.
		// TODO(wallyworld) - support resizing of existing storage.
		var unsupportedReason string
		if !reflect.DeepEqual(currentCharm.Meta().Deployment, newCharm.Meta().Deployment) {
			unsupportedReason = deploymentInfoUpgradeMessage
		} else if !reflect.DeepEqual(currentCharm.Meta().Storage, newCharm.Meta().Storage) {
			unsupportedReason = storageUpgradeMessage
		} else if !reflect.DeepEqual(currentCharm.Meta().Devices, newCharm.Meta().Devices) {
			unsupportedReason = devicesUpgradeMessage
		}
		if unsupportedReason != "" {
			return errors.NotSupportedf(unsupportedReason)
		}
		return api.applicationSetCharm(params, newCharm, stateCharmOrigin(newOrigin))
	}

	// Check if the controller agent tools version is greater than the
	// version we support for the new LXD profiles.
	// Then check all the units, to see what their agent tools versions is
	// so that we can ensure that everyone is aligned. If the units version
	// is too low (i.e. less than the 2.6.0 epoch), then show an error
	// message that the operator should upgrade to receive the latest
	// LXD Profile changes.

	// Ensure that we only check agent versions of a charm when we have a
	// non-empty profile. So this check will only be run in the following
	// scenarios; adding a profile, upgrading a profile. Removal of a
	// profile, that had an existing charm, will check if there is currently
	// an existing charm and if so, run the check.
	// Checking that is possible, but that would require asking every unit
	// machines what profiles they currently have and matching with the
	// incoming update. This could be very costly when you have lots of
	// machines.
	if lxdprofile.NotEmpty(lxdCharmProfiler{Charm: currentCharm}) ||
		lxdprofile.NotEmpty(lxdCharmProfiler{Charm: newCharm}) {
		if err := validateAgentVersions(oneApplication, api.model); err != nil {
			return errors.Trace(err)
		}
	}

	return api.applicationSetCharm(params, newCharm, stateCharmOrigin(newOrigin))
}

// applicationSetCharm sets the charm for the given for the application.
func (api *APIBase) applicationSetCharm(
	params setCharmParams,
	stateCharm Charm,
	stateOrigin *state.CharmOrigin,
) error {
	var err error
	var settings charm.Settings
	if params.ConfigSettingsYAML != "" {
		settings, err = stateCharm.Config().ParseSettingsYAML([]byte(params.ConfigSettingsYAML), params.AppName)
	} else if len(params.ConfigSettingsStrings) > 0 {
		settings, err = parseSettingsCompatible(stateCharm.Config(), params.ConfigSettingsStrings)
	}
	if err != nil {
		return errors.Annotate(err, "parsing config settings")
	}
	var stateStorageConstraints map[string]state.StorageConstraints
	if len(params.StorageConstraints) > 0 {
		stateStorageConstraints = make(map[string]state.StorageConstraints)
		for name, cons := range params.StorageConstraints {
			stateCons := state.StorageConstraints{Pool: cons.Pool}
			if cons.Size != nil {
				stateCons.Size = *cons.Size
			}
			if cons.Count != nil {
				stateCons.Count = *cons.Count
			}
			stateStorageConstraints[name] = stateCons
		}
	}
	force := params.Force
	cfg := state.SetCharmConfig{
		Charm:              api.stateCharm(stateCharm),
		CharmOrigin:        stateOrigin,
		Channel:            params.Channel,
		ConfigSettings:     settings,
		ForceSeries:        force.ForceSeries,
		ForceUnits:         force.ForceUnits,
		Force:              force.Force,
		ResourceIDs:        params.ResourceIDs,
		StorageConstraints: stateStorageConstraints,
		EndpointBindings:   params.EndpointBindings,
	}
	return params.Application.SetCharm(cfg)
}

// charmConfigFromYamlConfigValues will parse a yaml produced by juju get and
// generate charm.Settings from it that can then be sent to the application.
func charmConfigFromYamlConfigValues(yamlContents string) (charm.Settings, error) {
	var allSettings map[string]interface{}
	if err := goyaml.Unmarshal([]byte(yamlContents), &allSettings); err != nil {
		return nil, errors.Annotate(err, "cannot parse settings data")
	}
	return charmConfigFromConfigValues(allSettings)
}

// charmConfigFromConfigValues will parse a yaml produced by juju get and
// generate charm.Settings from it that can then be sent to the application.
func charmConfigFromConfigValues(yamlContents map[string]interface{}) (charm.Settings, error) {
	onlySettings := charm.Settings{}
	settingsMap, ok := yamlContents["settings"].(map[interface{}]interface{})
	if !ok {
		return nil, errors.New("unknown format for settings")
	}

	for setting := range settingsMap {
		s, ok := settingsMap[setting].(map[interface{}]interface{})
		if !ok {
			return nil, errors.Errorf("unknown format for settings section %v", setting)
		}
		// some keys might not have a value, we don't care about those.
		v, ok := s["value"]
		if !ok {
			continue
		}
		stringSetting, ok := setting.(string)
		if !ok {
			return nil, errors.Errorf("unexpected setting key, expected string got %T", setting)
		}
		onlySettings[stringSetting] = v
	}
	return onlySettings, nil
}

// GetCharmURL returns the charm URL the given application is
// running at present.
func (api *APIBase) GetCharmURL(args params.ApplicationGet) (params.StringResult, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.StringResult{}, errors.Trace(err)
	}
	oneApplication, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return params.StringResult{}, errors.Trace(err)
	}
	charmURL, _ := oneApplication.CharmURL()
	return params.StringResult{Result: charmURL.String()}, nil
}

// GetCharmURLOrigin isn't on the V12 API.
func (api *APIv12) GetCharmURLOrigin(_ struct{}) {}

// GetCharmURLOrigin returns the charm URL and charm origin the given
// application is running at present.
func (api *APIBase) GetCharmURLOrigin(args params.ApplicationGet) (params.CharmURLOriginResult, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.CharmURLOriginResult{}, errors.Trace(err)
	}
	oneApplication, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return params.CharmURLOriginResult{Error: apiservererrors.ServerError(err)}, nil
	}
	charmURL, _ := oneApplication.CharmURL()
	result := params.CharmURLOriginResult{URL: charmURL.String()}
	chOrigin := oneApplication.CharmOrigin()
	if chOrigin == nil {
		result.Error = apiservererrors.ServerError(errors.NotFoundf("charm origin for %q", args.ApplicationName))
		return result, nil
	}
	result.Origin = makeParamsCharmOrigin(chOrigin)
	return result, nil
}

func makeParamsCharmOrigin(origin *state.CharmOrigin) params.CharmOrigin {
	retOrigin := params.CharmOrigin{
		Source: origin.Source,
		ID:     origin.ID,
		Hash:   origin.Hash,
	}
	if origin.Revision != nil {
		retOrigin.Revision = origin.Revision
	}
	if origin.Channel != nil {
		retOrigin.Risk = origin.Channel.Risk
		if origin.Channel.Track != "" {
			retOrigin.Track = &origin.Channel.Track
		}
	}
	if origin.Platform != nil {
		retOrigin.Architecture = origin.Platform.Architecture
		retOrigin.OS = origin.Platform.OS
		retOrigin.Series = origin.Platform.Series
	}
	return retOrigin
}

// Set implements the server side of Application.Set.
// It does not unset values that are set to an empty string.
// Unset should be used for that.
func (api *APIBase) Set(p params.ApplicationSet) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	app, err := api.backend.Application(p.ApplicationName)
	if err != nil {
		return err
	}
	ch, _, err := app.Charm()
	if err != nil {
		return err
	}
	// Validate the settings.
	changes, err := ch.Config().ParseSettingsStrings(p.Options)
	if err != nil {
		return err
	}

	return app.UpdateCharmConfig(model.GenerationMaster, changes)
}

// Unset implements the server side of Client.Unset.
func (api *APIBase) Unset(p params.ApplicationUnset) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	app, err := api.backend.Application(p.ApplicationName)
	if err != nil {
		return err
	}
	settings := make(charm.Settings)
	for _, option := range p.Options {
		settings[option] = nil
	}

	// We need a guard on the API server-side for direct API callers such as
	// python-libjuju. Always default to the master branch.
	if p.BranchName == "" {
		p.BranchName = model.GenerationMaster
	}
	return app.UpdateCharmConfig(p.BranchName, settings)
}

// CharmRelations implements the server side of Application.CharmRelations.
func (api *APIBase) CharmRelations(p params.ApplicationCharmRelations) (params.ApplicationCharmRelationsResults, error) {
	var results params.ApplicationCharmRelationsResults
	if err := api.checkCanRead(); err != nil {
		return results, errors.Trace(err)
	}

	app, err := api.backend.Application(p.ApplicationName)
	if err != nil {
		return results, errors.Trace(err)
	}
	endpoints, err := app.Endpoints()
	if err != nil {
		return results, errors.Trace(err)
	}
	results.CharmRelations = make([]string, len(endpoints))
	for i, endpoint := range endpoints {
		results.CharmRelations[i] = endpoint.Relation.Name
	}
	return results, nil
}

// Expose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (api *APIBase) Expose(args params.ApplicationExpose) error {
	if err := api.checkCanWrite(); err != nil {
		return errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	if api.modelType == state.ModelTypeCAAS {
		appConfig, err := app.ApplicationConfig()
		if err != nil {
			return errors.Trace(err)
		}
		if appConfig.GetString(caas.JujuExternalHostNameKey, "") == "" {
			return errors.Errorf(
				"cannot expose a container application without a %q value set, run\n"+
					"juju config %s %s=<value>", caas.JujuExternalHostNameKey, args.ApplicationName, caas.JujuExternalHostNameKey)
		}
	}

	// Map space names to space IDs before calling SetExposed
	mappedExposeParams, err := api.mapExposedEndpointParams(args.ExposedEndpoints)
	if err != nil {
		return apiservererrors.ServerError(err)
	}

	// If an empty exposedEndpoints list is provided, all endpoints should
	// be exposed. This emulates the expose behavior of pre 2.9 controllers.
	if len(mappedExposeParams) == 0 {
		mappedExposeParams = map[string]state.ExposedEndpoint{
			"": {
				ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR},
			},
		}
	}

	if err = app.MergeExposeSettings(mappedExposeParams); err != nil {
		return apiservererrors.ServerError(err)
	}
	return nil
}

func (api *APIBase) mapExposedEndpointParams(params map[string]params.ExposedEndpoint) (map[string]state.ExposedEndpoint, error) {
	if len(params) == 0 {
		return nil, nil
	}

	var (
		spaceInfos network.SpaceInfos
		err        error
		res        = make(map[string]state.ExposedEndpoint, len(params))
	)

	for endpointName, exposeDetails := range params {
		mappedParam := state.ExposedEndpoint{
			ExposeToCIDRs: exposeDetails.ExposeToCIDRs,
		}

		if len(exposeDetails.ExposeToSpaces) != 0 {
			// Lazily fetch SpaceInfos
			if spaceInfos == nil {
				if spaceInfos, err = api.backend.AllSpaceInfos(); err != nil {
					return nil, err
				}
			}

			spaceIDs := make([]string, len(exposeDetails.ExposeToSpaces))
			for i, spaceName := range exposeDetails.ExposeToSpaces {
				sp := spaceInfos.GetByName(spaceName)
				if sp == nil {
					return nil, errors.NotFoundf("space %q", spaceName)
				}

				spaceIDs[i] = sp.ID
			}
			mappedParam.ExposeToSpaceIDs = spaceIDs
		}

		res[endpointName] = mappedParam

	}

	return res, nil
}

// Unexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (api *APIBase) Unexpose(args params.ApplicationUnexpose) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return err
	}

	// No endpoints specified; unexpose application
	if len(args.ExposedEndpoints) == 0 {
		return app.ClearExposed()
	}

	// Unset expose settings for the specified endpoints
	return app.UnsetExposeSettings(args.ExposedEndpoints)
}

// AddUnits adds a given number of units to an application.
func (api *APIv5) AddUnits(args params.AddApplicationUnitsV5) (params.AddApplicationUnitsResults, error) {
	noDefinedPolicy := ""
	return api.APIBase.AddUnits(params.AddApplicationUnits{
		ApplicationName: args.ApplicationName,
		NumUnits:        args.NumUnits,
		Placement:       args.Placement,
		Policy:          noDefinedPolicy,
		AttachStorage:   args.AttachStorage,
	})
}

// AddUnits adds a given number of units to an application.
func (api *APIBase) AddUnits(args params.AddApplicationUnits) (params.AddApplicationUnitsResults, error) {
	if api.modelType == state.ModelTypeCAAS {
		return params.AddApplicationUnitsResults{}, errors.NotSupportedf("adding units to a container-based model")
	}
	if err := api.checkCanWrite(); err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	units, err := addApplicationUnits(api.backend, api.modelType, args)
	if err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	unitNames := make([]string, len(units))
	for i, unit := range units {
		unitNames[i] = unit.UnitTag().Id()
	}
	return params.AddApplicationUnitsResults{Units: unitNames}, nil
}

// addApplicationUnits adds a given number of units to an application.
func addApplicationUnits(backend Backend, modelType state.ModelType, args params.AddApplicationUnits) ([]Unit, error) {
	if args.NumUnits < 1 {
		return nil, errors.New("must add at least one unit")
	}

	assignUnits := true
	if modelType != state.ModelTypeIAAS {
		// In a CAAS model, there are no machines for
		// units to be assigned to.
		assignUnits = false
		if len(args.AttachStorage) > 0 {
			return nil, errors.Errorf(
				"AttachStorage may not be specified for %s models",
				modelType,
			)
		}
		if len(args.Placement) > 1 {
			return nil, errors.Errorf(
				"only 1 placement directive is supported for %s models, got %d",
				modelType,
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
	oneApplication, err := backend.Application(args.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return addUnits(
		oneApplication,
		args.ApplicationName,
		args.NumUnits,
		args.Placement,
		attachStorage,
		assignUnits,
	)
}

// DestroyUnits removes a given set of application units.
//
// NOTE(axw) this exists only for backwards compatibility,
// for API facade versions 1-3; clients should prefer its
// successor, DestroyUnit, below. Until all consumers have
// been updated, or we bump a major version, we can't drop
// this.
//
// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (api *APIBase) DestroyUnits(args params.DestroyApplicationUnits) error {
	var errs []error
	entities := params.DestroyUnitsParams{
		Units: make([]params.DestroyUnitParams, 0, len(args.UnitNames)),
	}
	for _, unitName := range args.UnitNames {
		if !names.IsValidUnit(unitName) {
			errs = append(errs, errors.NotValidf("unit name %q", unitName))
			continue
		}
		entities.Units = append(entities.Units, params.DestroyUnitParams{
			UnitTag: names.NewUnitTag(unitName).String(),
		})
	}
	results, err := api.DestroyUnit(entities)
	if err != nil {
		return errors.Trace(err)
	}
	for _, result := range results.Results {
		if result.Error != nil {
			errs = append(errs, result.Error)
		}
	}
	return apiservererrors.DestroyErr("units", args.UnitNames, errs)
}

// DestroyUnit removes a given set of application units.
//
// NOTE(axw) this provides backwards compatibility for facade version 4.
func (api *APIv4) DestroyUnit(args params.Entities) (params.DestroyUnitResults, error) {
	v5args := params.DestroyUnitsParams{
		Units: make([]params.DestroyUnitParams, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		v5args.Units[i].UnitTag = arg.Tag
	}
	return api.APIBase.DestroyUnit(v5args)
}

// DestroyUnit removes a given set of application units.
func (api *APIBase) DestroyUnit(args params.DestroyUnitsParams) (params.DestroyUnitResults, error) {
	if api.modelType == state.ModelTypeCAAS {
		return params.DestroyUnitResults{}, errors.NotSupportedf("removing units on a non-container model")
	}
	if err := api.checkCanWrite(); err != nil {
		return params.DestroyUnitResults{}, errors.Trace(err)
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return params.DestroyUnitResults{}, errors.Trace(err)
	}
	destroyUnit := func(arg params.DestroyUnitParams) (*params.DestroyUnitInfo, error) {
		unitTag, err := names.ParseUnitTag(arg.UnitTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		name := unitTag.Id()
		unit, err := api.backend.Unit(name)
		if errors.IsNotFound(err) {
			return nil, errors.Errorf("unit %q does not exist", name)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if !unit.IsPrincipal() {
			return nil, errors.Errorf("unit %q is a subordinate", name)
		}
		var info params.DestroyUnitInfo
		unitStorage, err := storagecommon.UnitStorage(api.storageAccess, unit.UnitTag())
		if err != nil {
			return nil, errors.Trace(err)
		}

		if arg.DestroyStorage {
			for _, s := range unitStorage {
				info.DestroyedStorage = append(
					info.DestroyedStorage,
					params.Entity{Tag: s.StorageTag().String()},
				)
			}
		} else {
			info.DestroyedStorage, info.DetachedStorage, err = storagecommon.ClassifyDetachedStorage(
				api.storageAccess.VolumeAccess(), api.storageAccess.FilesystemAccess(), unitStorage,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		op := unit.DestroyOperation()
		op.DestroyStorage = arg.DestroyStorage
		op.Force = arg.Force
		if arg.Force {
			op.MaxWait = common.MaxWait(arg.MaxWait)
		}
		if err := api.backend.ApplyOperation(op); err != nil {
			return nil, errors.Trace(err)
		}
		if len(op.Errors) != 0 {
			logger.Warningf("operational errors destroying unit %v: %v", unit.Name(), op.Errors)
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

// Destroy destroys a given application, local or remote.
//
// NOTE(axw) this exists only for backwards compatibility,
// for API facade versions 1-3; clients should prefer its
// successor, DestroyApplication, below. Until all consumers
// have been updated, or we bump a major version, we can't
// drop this.
//
// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (api *APIBase) Destroy(in params.ApplicationDestroy) error {
	if !names.IsValidApplication(in.ApplicationName) {
		return errors.NotValidf("application name %q", in.ApplicationName)
	}
	args := params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: names.NewApplicationTag(in.ApplicationName).String(),
		}},
	}
	results, err := api.DestroyApplication(args)
	if err != nil {
		return errors.Trace(err)
	}
	if err := results.Results[0].Error; err != nil {
		return apiservererrors.ServerError(err)
	}
	return nil
}

// DestroyApplication removes a given set of applications.
//
// NOTE(axw) this provides backwards compatibility for facade version 4.
func (api *APIv4) DestroyApplication(args params.Entities) (params.DestroyApplicationResults, error) {
	v5args := params.DestroyApplicationsParams{
		Applications: make([]params.DestroyApplicationParams, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		v5args.Applications[i].ApplicationTag = arg.Tag
	}
	return api.APIBase.DestroyApplication(v5args)
}

// DestroyApplication removes a given set of applications.
func (api *APIBase) DestroyApplication(args params.DestroyApplicationsParams) (params.DestroyApplicationResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.DestroyApplicationResults{}, err
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return params.DestroyApplicationResults{}, errors.Trace(err)
	}
	destroyApp := func(arg params.DestroyApplicationParams) (*params.DestroyApplicationInfo, error) {
		tag, err := names.ParseApplicationTag(arg.ApplicationTag)
		if err != nil {
			return nil, err
		}
		var info params.DestroyApplicationInfo
		app, err := api.backend.Application(tag.Id())
		if err != nil {
			return nil, err
		}
		units, err := app.AllUnits()
		if err != nil {
			return nil, err
		}
		storageSeen := names.NewSet()
		for _, unit := range units {
			info.DestroyedUnits = append(
				info.DestroyedUnits,
				params.Entity{Tag: unit.UnitTag().String()},
			)
			unitStorage, err := storagecommon.UnitStorage(api.storageAccess, unit.UnitTag())
			if err != nil {
				return nil, err
			}

			// Filter out storage we've already seen. Shared
			// storage may be attached to multiple units.
			var unseen []state.StorageInstance
			for _, stor := range unitStorage {
				storageTag := stor.StorageTag()
				if storageSeen.Contains(storageTag) {
					continue
				}
				storageSeen.Add(storageTag)
				unseen = append(unseen, stor)
			}
			unitStorage = unseen

			if arg.DestroyStorage {
				for _, s := range unitStorage {
					info.DestroyedStorage = append(
						info.DestroyedStorage,
						params.Entity{Tag: s.StorageTag().String()},
					)
				}
			} else {
				destroyed, detached, err := storagecommon.ClassifyDetachedStorage(
					api.storageAccess.VolumeAccess(), api.storageAccess.FilesystemAccess(), unitStorage,
				)
				if err != nil {
					return nil, err
				}
				info.DestroyedStorage = append(info.DestroyedStorage, destroyed...)
				info.DetachedStorage = append(info.DetachedStorage, detached...)
			}
		}
		op := app.DestroyOperation()
		op.DestroyStorage = arg.DestroyStorage
		op.Force = arg.Force
		if arg.Force {
			op.MaxWait = common.MaxWait(arg.MaxWait)
		}
		if err := api.backend.ApplyOperation(op); err != nil {
			return nil, err
		}
		if len(op.Errors) != 0 {
			logger.Warningf("operational errors destroying application %v: %v", tag.Id(), op.Errors)
		}
		return &info, nil
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
func (api *APIBase) DestroyConsumedApplications(args params.DestroyConsumedApplicationsParams) (params.ErrorResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.ErrorResults{}, err
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	results := make([]params.ErrorResult, len(args.Applications))
	for i, arg := range args.Applications {
		appTag, err := names.ParseApplicationTag(arg.ApplicationTag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		app, err := api.backend.RemoteApplication(appTag.Id())
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		force := false
		if arg.Force != nil {
			force = *arg.Force
		}
		op := app.DestroyOperation(force)
		if force {
			op.MaxWait = common.MaxWait(arg.MaxWait)
		}
		err = api.backend.ApplyOperation(op)
		if op.Errors != nil && len(op.Errors) > 0 {
			logger.Warningf("operational error encountered destroying consumed application %v: %v", appTag.Id(), op.Errors)
		}
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return params.ErrorResults{
		Results: results,
	}, nil
}

// ScaleApplications isn't on the V7 API.
func (u *APIv7) ScaleApplications(_, _ struct{}) {}

// ScaleApplications scales the specified application to the requested number of units.
func (api *APIBase) ScaleApplications(args params.ScaleApplicationsParams) (params.ScaleApplicationResults, error) {
	if api.modelType != state.ModelTypeCAAS {
		return params.ScaleApplicationResults{}, errors.NotSupportedf("scaling applications on a non-container model")
	}
	if err := api.checkCanWrite(); err != nil {
		return params.ScaleApplicationResults{}, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return params.ScaleApplicationResults{}, errors.Trace(err)
	}
	scaleApplication := func(arg params.ScaleApplicationParams) (*params.ScaleApplicationInfo, error) {
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
		app, err := api.backend.Application(name)
		if errors.IsNotFound(err) {
			return nil, errors.Errorf("application %q does not exist", name)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		ch, _, err := app.Charm()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if ch.Meta().Deployment != nil {
			if ch.Meta().Deployment.DeploymentMode == charm.ModeOperator {
				return nil, errors.NotSupportedf("scale an %q application", charm.ModeOperator)
			}
			if ch.Meta().Deployment.DeploymentType == charm.DeploymentDaemon {
				return nil, errors.NotSupportedf("scale a %q application", charm.DeploymentDaemon)
			}
		}

		var info params.ScaleApplicationInfo
		if arg.ScaleChange != 0 {
			newScale, err := app.ChangeScale(arg.ScaleChange)
			if err != nil {
				return nil, errors.Trace(err)
			}
			info.Scale = newScale
		} else {
			if err := app.SetScale(arg.Scale, 0, true); err != nil {
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

// GetConstraints returns the constraints for a given application.
func (api *APIBase) GetConstraints(args params.Entities) (params.ApplicationGetConstraintsResults, error) {
	if err := api.checkCanRead(); err != nil {
		return params.ApplicationGetConstraintsResults{}, errors.Trace(err)
	}
	results := params.ApplicationGetConstraintsResults{
		Results: make([]params.ApplicationConstraint, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		cons, err := api.getConstraints(arg.Tag)
		results.Results[i].Constraints = cons
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (api *APIBase) getConstraints(entity string) (constraints.Value, error) {
	tag, err := names.ParseTag(entity)
	if err != nil {
		return constraints.Value{}, err
	}
	switch kind := tag.Kind(); kind {
	case names.ApplicationTagKind:
		app, err := api.backend.Application(tag.Id())
		if err != nil {
			return constraints.Value{}, err
		}
		return app.Constraints()
	default:
		return constraints.Value{}, errors.Errorf("unexpected tag type, expected application, got %s", kind)
	}
}

// SetConstraints sets the constraints for a given application.
func (api *APIBase) SetConstraints(args params.SetConstraints) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return err
	}
	return app.SetConstraints(args.Constraints)
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (api *APIBase) AddRelation(args params.AddRelation) (_ params.AddRelationResults, err error) {
	var rel Relation
	defer func() {
		if err != nil && rel != nil {
			if err := rel.Destroy(); err != nil {
				logger.Errorf("cannot destroy aborted relation %q: %v", rel.Tag().Id(), err)
			}
		}
	}()

	if err := api.check.ChangeAllowed(); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	if err := api.checkCanWrite(); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}

	// Validate any CIDRs.
	for _, cidr := range args.ViaCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return params.AddRelationResults{}, errors.Trace(err)
		}
		if cidr == "0.0.0.0/0" {
			return params.AddRelationResults{}, errors.Errorf("CIDR %q not allowed", cidr)
		}
	}

	inEps, err := api.backend.InferEndpoints(args.Endpoints...)
	if err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	if rel, err = api.backend.AddRelation(inEps...); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	if _, err := api.backend.SaveEgressNetworks(rel.Tag().Id(), args.ViaCIDRs); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}

	outEps := make(map[string]params.CharmRelation)
	for _, inEp := range inEps {
		outEp, err := rel.Endpoint(inEp.ApplicationName)
		if err != nil {
			return params.AddRelationResults{}, errors.Trace(err)
		}
		outEps[inEp.ApplicationName] = params.CharmRelation{
			Name:      outEp.Relation.Name,
			Role:      string(outEp.Relation.Role),
			Interface: outEp.Relation.Interface,
			Optional:  outEp.Relation.Optional,
			Limit:     outEp.Relation.Limit,
			Scope:     string(outEp.Relation.Scope),
		}
	}
	return params.AddRelationResults{Endpoints: outEps}, nil
}

// DestroyRelation removes the relation between the
// specified endpoints or an id.
func (api *APIBase) DestroyRelation(args params.DestroyRelation) (err error) {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return errors.Trace(err)
	}
	var (
		rel Relation
		eps []state.Endpoint
	)
	if len(args.Endpoints) > 0 {
		eps, err = api.backend.InferEndpoints(args.Endpoints...)
		if err != nil {
			return err
		}
		rel, err = api.backend.EndpointsRelation(eps...)
	} else {
		rel, err = api.backend.Relation(args.RelationId)
	}
	if err != nil {
		return err
	}
	force := args.Force != nil && *args.Force
	errs, err := rel.DestroyWithForce(force, common.MaxWait(args.MaxWait))
	if len(errs) != 0 {
		logger.Warningf("operational errors destroying relation %v: %v", rel.Tag().Id(), errs)
	}
	return err
}

// SetRelationsSuspended sets the suspended status of the specified relations.
func (api *APIBase) SetRelationsSuspended(args params.RelationSuspendedArgs) (params.ErrorResults, error) {
	var statusResults params.ErrorResults
	if err := api.checkCanWrite(); err != nil {
		return statusResults, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return statusResults, errors.Trace(err)
	}

	changeOne := func(arg params.RelationSuspendedArg) error {
		rel, err := api.backend.Relation(arg.RelationId)
		if err != nil {
			return errors.Trace(err)
		}
		if rel.Suspended() == arg.Suspended {
			return nil
		}
		_, err = api.backend.OfferConnectionForRelation(rel.Tag().Id())
		if errors.IsNotFound(err) {
			return errors.Errorf("cannot set suspend status for %q which is not associated with an offer", rel.Tag().Id())
		}
		message := arg.Message
		if !arg.Suspended {
			message = ""
		}
		err = rel.SetSuspended(arg.Suspended, message)
		if err != nil {
			return errors.Trace(err)
		}

		statusValue := status.Joining
		if arg.Suspended {
			statusValue = status.Suspending
		}
		return rel.SetStatus(status.StatusInfo{
			Status:  statusValue,
			Message: arg.Message,
		})
	}
	results := make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		err := changeOne(arg)
		results[i].Error = apiservererrors.ServerError(err)
	}
	statusResults.Results = results
	return statusResults, nil
}

// Consume adds remote applications to the model without creating any
// relations.
func (api *APIBase) Consume(args params.ConsumeApplicationArgs) (params.ErrorResults, error) {
	var consumeResults params.ErrorResults
	if err := api.checkCanWrite(); err != nil {
		return consumeResults, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return consumeResults, errors.Trace(err)
	}

	results := make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		err := api.consumeOne(arg)
		results[i].Error = apiservererrors.ServerError(err)
	}
	consumeResults.Results = results
	return consumeResults, nil
}

func (api *APIBase) consumeOne(arg params.ConsumeApplicationArg) error {
	sourceModelTag, err := names.ParseModelTag(arg.SourceModelTag)
	if err != nil {
		return errors.Trace(err)
	}

	// Maybe save the details of the controller hosting the offer.
	if arg.ControllerInfo != nil {
		controllerTag, err := names.ParseControllerTag(arg.ControllerInfo.ControllerTag)
		if err != nil {
			return errors.Trace(err)
		}
		// Only save controller details if the offer comes from
		// a different controller.
		if controllerTag.Id() != api.backend.ControllerTag().Id() {
			if _, err = api.backend.SaveController(crossmodel.ControllerInfo{
				ControllerTag: controllerTag,
				Alias:         arg.ControllerInfo.Alias,
				Addrs:         arg.ControllerInfo.Addrs,
				CACert:        arg.ControllerInfo.CACert,
			}, sourceModelTag.Id()); err != nil {
				return errors.Trace(err)
			}
		}
	}

	appName := arg.ApplicationAlias
	if appName == "" {
		appName = arg.OfferName
	}
	_, err = api.saveRemoteApplication(sourceModelTag, appName, arg.ApplicationOfferDetails, arg.Macaroon)
	return err
}

// saveRemoteApplication saves the details of the specified remote application and its endpoints
// to the state model so relations to the remote application can be created.
func (api *APIBase) saveRemoteApplication(
	sourceModelTag names.ModelTag,
	applicationName string,
	offer params.ApplicationOfferDetails,
	mac *macaroon.Macaroon,
) (RemoteApplication, error) {
	remoteEps := make([]charm.Relation, len(offer.Endpoints))
	for j, ep := range offer.Endpoints {
		remoteEps[j] = charm.Relation{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
		}
	}

	remoteSpaces := make([]*environs.ProviderSpaceInfo, len(offer.Spaces))
	for i, space := range offer.Spaces {
		remoteSpaces[i] = providerSpaceInfoFromParams(space)
	}

	// If the a remote application with the same name and endpoints from the same
	// source model already exists, we will use that one.
	remoteApp, err := api.maybeUpdateExistingApplicationEndpoints(applicationName, sourceModelTag, remoteEps)
	if err == nil {
		return remoteApp, nil
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	return api.backend.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        applicationName,
		OfferUUID:   offer.OfferUUID,
		URL:         offer.OfferURL,
		SourceModel: sourceModelTag,
		Endpoints:   remoteEps,
		Spaces:      remoteSpaces,
		Bindings:    offer.Bindings,
		Macaroon:    mac,
	})
}

// providerSpaceInfoFromParams converts a params.RemoteSpace to the
// equivalent ProviderSpaceInfo.
func providerSpaceInfoFromParams(space params.RemoteSpace) *environs.ProviderSpaceInfo {
	result := &environs.ProviderSpaceInfo{
		CloudType:          space.CloudType,
		ProviderAttributes: space.ProviderAttributes,
		SpaceInfo: network.SpaceInfo{
			Name:       network.SpaceName(space.Name),
			ProviderId: network.Id(space.ProviderId),
		},
	}
	for _, subnet := range space.Subnets {
		resultSubnet := network.SubnetInfo{
			CIDR:              subnet.CIDR,
			ProviderId:        network.Id(subnet.ProviderId),
			ProviderNetworkId: network.Id(subnet.ProviderNetworkId),
			ProviderSpaceId:   network.Id(subnet.ProviderSpaceId),
			VLANTag:           subnet.VLANTag,
			AvailabilityZones: subnet.Zones,
		}
		result.Subnets = append(result.Subnets, resultSubnet)
	}
	return result
}

// maybeUpdateExistingApplicationEndpoints looks for a remote application with the
// specified name and source model tag and tries to update its endpoints with the
// new ones specified. If the endpoints are compatible, the newly updated remote
// application is returned.
func (api *APIBase) maybeUpdateExistingApplicationEndpoints(
	applicationName string, sourceModelTag names.ModelTag, remoteEps []charm.Relation,
) (RemoteApplication, error) {
	existingRemoteApp, err := api.backend.RemoteApplication(applicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if existingRemoteApp.SourceModel().Id() != sourceModelTag.Id() {
		return nil, errors.AlreadyExistsf("remote application called %q from a different model", applicationName)
	}
	newEpsMap := make(map[charm.Relation]bool)
	for _, ep := range remoteEps {
		newEpsMap[ep] = true
	}
	existingEps, err := existingRemoteApp.Endpoints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	maybeSameEndpoints := len(newEpsMap) == len(existingEps)
	existingEpsByName := make(map[string]charm.Relation)
	for _, ep := range existingEps {
		existingEpsByName[ep.Name] = ep.Relation
		delete(newEpsMap, ep.Relation)
	}
	sameEndpoints := maybeSameEndpoints && len(newEpsMap) == 0
	if sameEndpoints {
		return existingRemoteApp, nil
	}

	// Gather the new endpoints. All new endpoints passed to AddEndpoints()
	// below must not have the same name as an existing endpoint.
	var newEps []charm.Relation
	for ep := range newEpsMap {
		// See if we are attempting to update endpoints with the same name but
		// different relation data.
		if existing, ok := existingEpsByName[ep.Name]; ok && existing != ep {
			return nil, errors.Errorf("conflicting endpoint %v", ep.Name)
		}
		newEps = append(newEps, ep)
	}

	if len(newEps) > 0 {
		// Update the existing remote app to have the new, additional endpoints.
		if err := existingRemoteApp.AddEndpoints(newEps); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return existingRemoteApp, nil
}

// Mask the new methods from the V4 API. The API reflection code in
// rpc/rpcreflect/type.go:newMethod skips 2-argument methods, so this
// removes the method as far as the RPC machinery is concerned.

// UpdateApplicationSeries isn't on the V4 API.
func (u *APIv4) UpdateApplicationSeries(_, _ struct{}) {}

// GetConfig isn't on the V4 API.
func (u *APIv4) GetConfig(_, _ struct{}) {}

// GetConstraints returns the v4 implementation of GetConstraints.
func (api *APIv4) GetConstraints(args params.GetApplicationConstraints) (params.GetConstraintsResults, error) {
	if err := api.checkCanRead(); err != nil {
		return params.GetConstraintsResults{}, errors.Trace(err)
	}
	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return params.GetConstraintsResults{}, errors.Trace(err)
	}
	cons, err := app.Constraints()
	return params.GetConstraintsResults{
		Constraints: cons,
	}, errors.Trace(err)
}

// Mask the new methods from the v4 and v5 API. The API reflection code in
// rpc/rpcreflect/type.go:newMethod skips 2-argument methods, so this
// removes the method as far as the RPC machinery is concerned.
//
// Since the v4 builds on v5, we can just make the methods unavailable on v5
// and they will also be unavailable on v4.

// CharmConfig isn't on the v5 API.
func (u *APIv5) CharmConfig(_, _ struct{}) {}

// CharmConfig is a shim to GetConfig on APIv5. It returns only charm config.
// Version 8 and below accept params.Entities, where later versions must accept
// a model generation
func (api *APIv8) CharmConfig(args params.Entities) (params.ApplicationGetConfigResults, error) {
	return api.GetConfig(args)
}

// CharmConfig returns charm config for the input list of applications and
// model generations.
func (api *APIBase) CharmConfig(args params.ApplicationGetArgs) (params.ApplicationGetConfigResults, error) {
	if err := api.checkCanRead(); err != nil {
		return params.ApplicationGetConfigResults{}, err
	}
	results := params.ApplicationGetConfigResults{
		Results: make([]params.ConfigResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		config, err := api.getCharmConfig(arg.BranchName, arg.ApplicationName)
		results.Results[i].Config = config
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// GetConfig returns the charm config for each of the input applications.
func (api *APIBase) GetConfig(args params.Entities) (params.ApplicationGetConfigResults, error) {
	if err := api.checkCanRead(); err != nil {
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
		config, err := api.getCharmConfig(model.GenerationMaster, tag.Id())
		results.Results[i].Config = config
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (api *APIBase) getCharmConfig(gen string, appName string) (map[string]interface{}, error) {
	app, err := api.backend.Application(appName)
	if err != nil {
		return nil, err
	}
	settings, err := app.CharmConfig(gen)
	if err != nil {
		return nil, err
	}
	ch, _, err := app.Charm()
	if err != nil {
		return nil, err
	}
	return describe(settings, ch.Config()), nil
}

// SetApplicationsConfig isn't on the v5 API.
func (u *APIv5) SetApplicationsConfig(_, _ struct{}) {}

// SetApplicationsConfig implements the server side of Application.SetApplicationsConfig.
// It does not unset values that are set to an empty string.
// Unset should be used for that.
// Note: SetApplicationsConfig is misleading, both application and charm config are set.
// Note: For facade version 13 and higher, use SetConfig.
func (api *APIv12) SetApplicationsConfig(args params.ApplicationConfigSetArgs) (params.ErrorResults, error) {
	var result params.ErrorResults
	if err := api.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return result, errors.Trace(err)
	}
	result.Results = make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		app, err := api.backend.Application(arg.ApplicationName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		err = api.setConfig(app, arg.Generation, "", arg.Config)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// SetConfig implements the server side of Application.SetConfig.  Both
// application and charm config are set. It does not unset values in
// Config map that are set to an empty string. Unset should be used for that.
func (api *APIBase) SetConfigs(args params.ConfigSetArgs) (params.ErrorResults, error) {
	var result params.ErrorResults
	if err := api.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return result, errors.Trace(err)
	}
	result.Results = make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		app, err := api.backend.Application(arg.ApplicationName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = api.setConfig(app, arg.Generation, arg.ConfigYAML, arg.Config)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (api *APIBase) addAppToBranch(branchName string, appName string) error {
	gen, err := api.backend.Branch(branchName)
	if err != nil {
		return errors.Annotate(err, "retrieving next generation")
	}
	err = gen.AssignApplication(appName)
	return errors.Annotatef(err, "adding %q to next generation", appName)
}

// UnsetApplicationsConfig isn't on the v5 API.
func (u *APIv5) UnsetApplicationsConfig(_, _ struct{}) {}

// UnsetApplicationsConfig implements the server side of Application.UnsetApplicationsConfig.
func (api *APIBase) UnsetApplicationsConfig(args params.ApplicationConfigUnsetArgs) (params.ErrorResults, error) {
	var result params.ErrorResults
	if err := api.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return result, errors.Trace(err)
	}
	result.Results = make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		err := api.unsetApplicationConfig(arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (api *APIBase) unsetApplicationConfig(arg params.ApplicationUnset) error {
	app, err := api.backend.Application(arg.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}

	configSchema, defaults, err := applicationConfigSchema(api.modelType)
	if err != nil {
		return errors.Trace(err)
	}
	appConfigFields := application.KnownConfigKeys(configSchema)

	var appConfigKeys []string
	charmSettings := make(charm.Settings)
	for _, name := range arg.Options {
		if appConfigFields.Contains(name) {
			appConfigKeys = append(appConfigKeys, name)
		} else {
			charmSettings[name] = nil
		}
	}

	if len(appConfigKeys) > 0 {
		if err := app.UpdateApplicationConfig(nil, appConfigKeys, configSchema, defaults); err != nil {
			return errors.Annotate(err, "updating application config values")
		}
	}

	if len(charmSettings) > 0 {
		// We need a guard on the API server-side for direct API callers such as
		// python-libjuju, and for older clients.
		// Always default to the master branch.
		if arg.BranchName == "" {
			arg.BranchName = model.GenerationMaster
		}
		if err := app.UpdateCharmConfig(arg.BranchName, charmSettings); err != nil {
			return errors.Annotate(err, "updating application charm settings")
		}
	}
	return nil
}

// ResolveUnitErrors isn't on the v5 API.
func (u *APIv5) ResolveUnitErrors(_, _ struct{}) {}

// ResolveUnitErrors marks errors on the specified units as resolved.
func (api *APIBase) ResolveUnitErrors(p params.UnitsResolved) (params.ErrorResults, error) {
	if p.All {
		unitsWithErrors, err := api.backend.UnitsInError()
		if err != nil {
			return params.ErrorResults{}, errors.Trace(err)
		}
		for _, u := range unitsWithErrors {
			if err := u.Resolve(p.Retry); err != nil {
				return params.ErrorResults{}, errors.Annotatef(err, "resolve error for unit %q", u.UnitTag().Id())
			}
		}
	}

	var result params.ErrorResults
	if err := api.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return result, errors.Trace(err)
	}

	result.Results = make([]params.ErrorResult, len(p.Tags.Entities))
	for i, entity := range p.Tags.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		unit, err := api.backend.Unit(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = unit.Resolve(p.Retry)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// ApplicationInfo isn't on the v8 API.
func (u *APIv8) ApplicationInfo(_, _ struct{}) {}

// ApplicationsInfo returns applications information.
func (api *APIBase) ApplicationsInfo(in params.Entities) (params.ApplicationInfoResults, error) {
	// Get all the space infos before iterating over the application infos.
	allSpaceInfosLookup, err := api.backend.AllSpaceInfos()
	if err != nil {
		return params.ApplicationInfoResults{}, apiservererrors.ServerError(err)
	}

	out := make([]params.ApplicationInfoResult, len(in.Entities))
	for i, one := range in.Entities {
		tag, err := names.ParseApplicationTag(one.Tag)
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}
		app, err := api.backend.Application(tag.Name)
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		details, err := api.getConfig(params.ApplicationGet{ApplicationName: tag.Name}, describe)
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		bindings, err := app.EndpointBindings()
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		bindingsMap, err := bindings.MapWithSpaceNames(allSpaceInfosLookup)
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		exposedEndpoints, err := api.mapExposedEndpointsFromState(app.ExposedEndpoints())
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		var channel string
		origin := app.CharmOrigin()
		if origin != nil && origin.Channel != nil {
			ch := origin.Channel
			channel = charm.MakePermissiveChannel(ch.Track, ch.Risk, ch.Branch).String()
		} else {
			channel = details.Channel
		}

		out[i].Result = &params.ApplicationResult{
			Tag:              tag.String(),
			Charm:            details.Charm,
			Series:           details.Series,
			Channel:          channel,
			Constraints:      details.Constraints,
			Principal:        app.IsPrincipal(),
			Exposed:          app.IsExposed(),
			Remote:           app.IsRemote(),
			EndpointBindings: bindingsMap,
			ExposedEndpoints: exposedEndpoints,
		}
	}
	return params.ApplicationInfoResults{
		Results: out,
	}, nil
}

func (api *APIBase) mapExposedEndpointsFromState(exposedEndpoints map[string]state.ExposedEndpoint) (map[string]params.ExposedEndpoint, error) {
	if len(exposedEndpoints) == 0 {
		return nil, nil
	}

	var (
		spaceInfos network.SpaceInfos
		err        error
		res        = make(map[string]params.ExposedEndpoint, len(exposedEndpoints))
	)

	for endpointName, exposeDetails := range exposedEndpoints {
		mappedParam := params.ExposedEndpoint{
			ExposeToCIDRs: exposeDetails.ExposeToCIDRs,
		}

		if len(exposeDetails.ExposeToSpaceIDs) != 0 {
			// Lazily fetch SpaceInfos
			if spaceInfos == nil {
				if spaceInfos, err = api.backend.AllSpaceInfos(); err != nil {
					return nil, err
				}
			}

			spaceNames := make([]string, len(exposeDetails.ExposeToSpaceIDs))
			for i, spaceID := range exposeDetails.ExposeToSpaceIDs {
				sp := spaceInfos.GetByID(spaceID)
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
func (api *APIBase) MergeBindings(in params.ApplicationMergeBindingsArgs) (params.ErrorResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.ErrorResults{}, err
	}

	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	res := make([]params.ErrorResult, len(in.Args))
	for i, arg := range in.Args {
		tag, err := names.ParseApplicationTag(arg.ApplicationTag)
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}
		app, err := api.backend.Application(tag.Name)
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		bindings, err := state.NewBindings(api.backend, arg.Bindings)
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if err := app.MergeBindings(bindings, arg.Force); err != nil {
			res[i].Error = apiservererrors.ServerError(err)
		}
	}
	return params.ErrorResults{Results: res}, nil
}

// lxdCharmProfiler massages a *state.Charm into a LXDProfiler
// inside of the core package.
type lxdCharmProfiler struct {
	Charm Charm
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p lxdCharmProfiler) LXDProfile() lxdprofile.LXDProfile {
	if p.Charm == nil {
		return nil
	}
	if profiler, ok := p.Charm.(charm.LXDProfiler); ok {
		profile := profiler.LXDProfile()
		if profile == nil {
			return nil
		}
		return profile
	}
	return nil
}

// AgentTools is a point of use agent tools requester.
type AgentTools interface {
	AgentTools() (*tools.Tools, error)
}

// AgentVersioner is a point of use agent version object.
type AgentVersioner interface {
	AgentVersion() (version.Number, error)
}

var (
	// ErrInvalidAgentVersions is a sentinal error for when we can no longer
	// upgrade juju using 2.5.x agents with 2.6 or greater controllers.
	ErrInvalidAgentVersions = errors.Errorf(
		"Unable to upgrade LXDProfile charms with the current model version. " +
			"Please run juju upgrade-juju to upgrade the current model to match your controller.")
)

func getAgentToolsVersion(agentTools AgentTools) (version.Number, error) {
	tools, err := agentTools.AgentTools()
	if err != nil {
		return version.Zero, err
	}
	return tools.Version.Number, nil
}

func getAgentVersion(versioner AgentVersioner) (version.Number, error) {
	agent, err := versioner.AgentVersion()
	if err != nil {
		return version.Zero, err
	}
	return agent, nil
}

func validateAgentVersions(application Application, versioner AgentVersioner) error {
	// The epoch is set like this, because beta tags are less than release tags.
	// So 2.6-beta1.1 < 2.6.0, even though the patch is greater than 0. To
	// prevent the miss-match, we add the upper epoch limit.
	epoch := version.Number{Major: 2, Minor: 5, Patch: math.MaxInt32}

	// Locate the agent tools version to limit the amount of checking we
	// required to do over all. We check for NotFound to also use that as a
	// fallthrough to check the agent version as well. This should take care
	// of places where the application.AgentTools version is not set (IAAS).
	ver, err := getAgentToolsVersion(application)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if errors.IsNotFound(err) || ver.Compare(epoch) >= 0 {
		// Check to see if the model config version is valid
		// Arguably we could check on the per-unit level, as that is the
		// *actual* version of the agent that is running, looking at the
		// versioner (alias to model config), we get the intent of the move
		// to that version.
		// This should be enough for a pre-flight check, rather than querying
		// potentially thousands of units (think large production stacks).
		modelVer, modelErr := getAgentVersion(versioner)
		if modelErr != nil {
			// If we can't find the model config version, then we can't do the
			// comparison check.
			return errors.Trace(modelErr)
		}
		if modelVer.Compare(epoch) < 0 {
			return ErrInvalidAgentVersions
		}
	}
	return nil
}

// UnitsInfo isn't on the v11 API.
func (u *APIv11) UnitsInfo(_, _ struct{}) {}

// UnitsInfo returns unit information.
func (api *APIBase) UnitsInfo(in params.Entities) (params.UnitInfoResults, error) {
	out := make([]params.UnitInfoResult, len(in.Entities))
	leaders, err := api.leadershipReader.Leaders()
	if err != nil {
		return params.UnitInfoResults{}, errors.Trace(err)
	}
	for i, one := range in.Entities {
		tag, err := names.ParseUnitTag(one.Tag)
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}
		unit, err := api.backend.Unit(tag.Id())
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}
		app, err := api.backend.Application(unit.ApplicationName())
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}
		curl, _ := app.CharmURL()
		machineId, _ := unit.AssignedMachineId()
		workloadVersion, err := unit.WorkloadVersion()
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result := &params.UnitResult{
			Tag:             tag.String(),
			WorkloadVersion: workloadVersion,
			Machine:         machineId,
			Charm:           curl.String(),
		}
		if leader := leaders[unit.ApplicationName()]; leader == unit.Name() {
			result.Leader = true
		}
		if machineId != "" {
			machine, err := api.backend.Machine(machineId)
			if err != nil {
				out[i].Error = apiservererrors.ServerError(err)
				continue
			}
			publicAddress, err := machine.PublicAddress()
			if err == nil {
				result.PublicAddress = publicAddress.Value
			}
			// NOTE(achilleasa): this call completely ignores
			// subnets and lumps all port ranges together in a
			// single group. This works fine for pre 2.9 agents
			// as ports where always opened across all subnets.
			openPorts, err := api.openPortsOnMachineForUnit(unit.Name(), machineId)
			if err != nil {
				out[i].Error = apiservererrors.ServerError(err)
				continue
			}
			result.OpenedPorts = openPorts
		}
		container, err := unit.ContainerInfo()
		if err != nil && !errors.IsNotFound(err) {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err == nil {
			if addr := container.Address(); addr != nil {
				result.Address = addr.Value
			}
			result.ProviderId = container.ProviderId()
			if len(result.OpenedPorts) == 0 {
				result.OpenedPorts = container.Ports()
			}
		}
		result.RelationData, err = api.relationData(app, unit)
		if err != nil {
			out[i].Error = apiservererrors.ServerError(err)
			continue
		}

		out[i].Result = result
	}
	return params.UnitInfoResults{
		Results: out,
	}, nil
}

// openPortsOnMachineForUnit returns the unique set of opened ports for the
// specified unit and machine arguments without distinguishing between port
// ranges across subnets. This method is provided for backwards compatibility
// with pre 2.9 agents which assume open-ports apply to all subnets.
func (api *APIBase) openPortsOnMachineForUnit(unitName, machineID string) ([]string, error) {
	var result []string
	machinePortRanges, err := api.model.OpenedPortRangesForMachine(machineID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, portRange := range machinePortRanges.ForUnit(unitName).UniquePortRanges() {
		result = append(result, portRange.String())
	}
	return result, nil
}

func (api *APIBase) relationData(app Application, myUnit Unit) ([]params.EndpointRelationData, error) {
	rels, err := app.Relations()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]params.EndpointRelationData, len(rels))
	for i, rel := range rels {
		ep, err := rel.Endpoint(app.Name())
		if err != nil {
			return nil, errors.Trace(err)
		}
		erd := params.EndpointRelationData{
			Endpoint:         ep.Name,
			ApplicationData:  make(map[string]interface{}),
			UnitRelationData: make(map[string]params.RelationData),
		}
		appSettings, err := rel.ApplicationSettings(app.Name())
		if err != nil {
			return nil, errors.Trace(err)
		}
		for k, v := range appSettings {
			erd.ApplicationData[k] = v
		}
		relatedEps, err := rel.RelatedEndpoints(app.Name())
		if err != nil {
			return nil, errors.Trace(err)
		}
		// There is only one related endpoint.
		related := relatedEps[0]
		erd.RelatedEndpoint = related.Name

		otherApp, err := api.backend.Application(related.ApplicationName)
		if errors.IsNotFound(err) {
			erd.CrossModel = true
			if err := api.crossModelRelationData(rel, related.ApplicationName, &erd); err != nil {
				return nil, errors.Trace(err)
			}
			result[i] = erd
			continue
		}
		if err != nil {
			return nil, errors.Trace(err)
		}

		otherUnits, err := otherApp.AllUnits()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, u := range otherUnits {
			ru, err := rel.Unit(u.Name())
			if err != nil {
				return nil, errors.Trace(err)
			}
			inScope, err := ru.InScope()
			if err != nil {
				return nil, errors.Trace(err)
			}
			urd := params.RelationData{
				InScope: inScope,
			}
			if inScope {
				settings, err := ru.Settings()
				if err != nil && !errors.IsNotFound(err) {
					return nil, errors.Trace(err)
				}
				if err == nil {
					urd.UnitData = make(map[string]interface{})
					for k, v := range settings {
						urd.UnitData[k] = v
					}
				}
			}
			erd.UnitRelationData[u.Name()] = urd
		}

		result[i] = erd
	}
	return result, nil
}

func (api *APIBase) crossModelRelationData(rel Relation, appName string, erd *params.EndpointRelationData) error {
	rus, err := rel.AllRemoteUnits(appName)
	if err != nil {
		return errors.Trace(err)
	}
	for _, ru := range rus {
		inScope, err := ru.InScope()
		if err != nil {
			return errors.Trace(err)
		}
		urd := params.RelationData{
			InScope: inScope,
		}
		if inScope {
			settings, err := ru.Settings()
			if err != nil && !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			if err == nil {
				urd.UnitData = make(map[string]interface{})
				for k, v := range settings {
					urd.UnitData[k] = v
				}
			}
		}
		erd.UnitRelationData[ru.UnitName()] = urd
	}
	return nil
}
