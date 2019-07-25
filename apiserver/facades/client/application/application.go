// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package application contains api calls for functionality
// related to deploying and managing applications and their
// related charms.
package application

import (
	"fmt"
	"math"
	"net"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/schema"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6"
	csparams "gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v2-unstable"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/tools"
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
	*APIBase
}

// APIBase implements the shared application interface and is the concrete
// implementation of the api end point.
//
// API provides the Application API facade for version 5.
type APIBase struct {
	backend       Backend
	storageAccess storageInterface

	authorizer facade.Authorizer
	check      BlockChecker

	model     Model
	modelType state.ModelType

	resources facade.Resources

	// TODO(axw) stateCharm only exists because I ran out
	// of time unwinding all of the tendrils of state. We
	// should pass a charm.Charm and charm.URL back into
	// state wherever we pass in a state.Charm currently.
	stateCharm func(Charm) *state.Charm

	storagePoolManager    poolmanager.PoolManager
	registry              storage.ProviderRegistry
	storageValidator      caas.StorageValidator
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
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv10{api}, nil
}

func newFacadeBase(ctx facade.Context) (*APIBase, error) {
	facadeModel, err := ctx.State().Model()
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
		storageValidator   caas.Broker
	)
	if facadeModel.Type() == state.ModelTypeCAAS {
		storageValidator, err = stateenvirons.GetNewCAASBrokerFunc(caas.New)(ctx.State())
		if err != nil {
			return nil, errors.Annotate(err, "getting caas client")
		}
		registry = stateenvirons.NewStorageProviderRegistry(storageValidator)
		storagePoolManager = poolmanager.New(state.NewStateSettings(ctx.State()), registry)
	}

	resources := ctx.Resources()

	return NewAPIBase(
		&stateShim{ctx.State()},
		storageAccess,
		ctx.Auth(),
		blockChecker,
		facadeModel,
		stateCharm,
		DeployApplication,
		storagePoolManager,
		registry,
		resources,
		storageValidator,
	)
}

// NewAPIBase returns a new application API facade.
func NewAPIBase(
	backend Backend,
	storageAccess storageInterface,
	authorizer facade.Authorizer,
	blockChecker BlockChecker,
	model Model,
	stateCharm func(Charm) *state.Charm,
	deployApplication func(ApplicationDeployer, DeployApplicationParams) (Application, error),
	storagePoolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
	resources facade.Resources,
	storageValidator caas.StorageValidator,
) (*APIBase, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &APIBase{
		backend:               backend,
		storageAccess:         storageAccess,
		authorizer:            authorizer,
		check:                 blockChecker,
		model:                 model,
		modelType:             model.Type(),
		stateCharm:            stateCharm,
		deployApplicationFunc: deployApplication,
		storagePoolManager:    storagePoolManager,
		registry:              registry,
		resources:             resources,
		storageValidator:      storageValidator,
	}, nil
}

func (api *APIBase) checkPermission(tag names.Tag, perm permission.Access) error {
	allowed, err := api.authorizer.HasPermission(perm, tag)
	if err != nil {
		return errors.Trace(err)
	}
	if !allowed {
		return common.ErrPerm
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
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = oneApplication.SetMetricCredentials(a.MetricCredentials)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
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
		err := deployApplication(api.backend, api.model, api.stateCharm, arg, api.deployApplicationFunc, api.storagePoolManager, api.registry, api.storageValidator)
		result.Results[i].Error = common.ServerError(err)

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
	var allSettings map[string]map[string]interface{}
	if err := goyaml.Unmarshal([]byte(inYaml), &allSettings); err != nil {
		return nil, "", errors.Annotate(err, "cannot parse settings data")
	}
	settings, ok := allSettings[appName]
	if !ok {
		return nil, "", errors.Errorf("no settings found for %q", appName)
	}

	providerSchema, _, err := applicationConfigSchema(modelType)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	appConfigKeys := application.KnownConfigKeys(providerSchema)

	appConfigAttrs := make(map[string]interface{})
	for k, v := range settings {
		if appConfigKeys.Contains(k) {
			appConfigAttrs[k] = v
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
	storageValidator caas.StorageValidator,
) error {
	curl, err := charm.ParseURL(args.CharmURL)
	if err != nil {
		return errors.Trace(err)
	}
	if curl.Revision < 0 {
		return errors.Errorf("charm url must include revision")
	}

	modelType := model.Type()
	if modelType != state.ModelTypeIAAS {
		if len(args.AttachStorage) > 0 {
			return errors.Errorf(
				"AttachStorage may not be specified for %s models",
				modelType,
			)
		}
		if len(args.Placement) > 1 {
			return errors.Errorf(
				"only 1 placement directive is supported for %s models, got %d",
				modelType,
				len(args.Placement),
			)
		}

		cfg, err := model.ModelConfig()
		if err != nil {
			return errors.Trace(err)
		}
		storageClassName, _ := cfg.AllAttrs()[k8s.OperatorStorageKey].(string)
		if storageClassName == "" {
			return errors.New(
				"deploying a Kubernetes application requires a suitable storage class.\n" +
					"None have been configured. Set the operator-storage model config to " +
					"specify which storage class should be used to allocate operator storage.\n" +
					"See https://discourse.jujucharms.com/t/getting-started/152.",
			)
		}
		sp, err := caasoperatorprovisioner.CharmStorageParams("", storageClassName, cfg, "", storagePoolManager, registry)
		if err != nil {
			return errors.Annotatef(err, "getting operator storage params for %q", args.ApplicationName)
		}
		if sp.Provider != string(k8s.K8s_ProviderType) {
			poolName := cfg.AllAttrs()[k8s.OperatorStorageKey]
			return errors.Errorf(
				"the %q storage pool requires a provider type of %q, not %q", poolName, k8s.K8s_ProviderType, sp.Provider)
		}
		if err := storageValidator.ValidateStorageClass(sp.Attributes); err != nil {
			return errors.Trace(err)
		}

		workloadStorageClass, _ := cfg.AllAttrs()[k8s.WorkloadStorageKey].(string)
		for storageName, cons := range args.Storage {
			if cons.Pool == "" && workloadStorageClass == "" {
				return errors.Errorf("storage pool for %q must be specified since there's no model default storage class", storageName)
			}
			_, err := caasoperatorprovisioner.CharmStorageParams("", workloadStorageClass, cfg, cons.Pool, storagePoolManager, registry)
			if err != nil {
				return errors.Annotatef(err, "getting workload storage params for %q", args.ApplicationName)
			}
		}
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

	if err := checkMinVersion(ch); err != nil {
		return errors.Trace(err)
	}

	// Split out the app config from the charm config for any config
	// passed in as a map as opposed to YAML.
	var appConfig map[string]interface{}
	var charmConfig map[string]string
	if len(args.Config) > 0 {
		if appConfig, charmConfig, err = splitApplicationAndCharmConfig(modelType, args.Config); err != nil {
			return errors.Trace(err)
		}
	}

	// Split out the app config from the charm config for any config
	// passed in as YAML.
	var charmYamlConfig string
	appSettings := make(map[string]interface{})
	if len(args.ConfigYAML) > 0 {
		if appSettings, charmYamlConfig, err = splitApplicationAndCharmConfigFromYAML(modelType, args.ConfigYAML, args.ApplicationName); err != nil {
			return errors.Trace(err)
		}
	}

	// Overlay any app settings in YAML with those from config map.
	for k, v := range appConfig {
		appSettings[k] = v
	}

	var applicationConfig *application.Config
	configSchema, defaults, err := applicationConfigSchema(modelType)
	if err != nil {
		return errors.Trace(err)
	}
	applicationConfig, err = application.NewConfig(appSettings, configSchema, defaults)
	if err != nil {
		return errors.Trace(err)
	}

	var settings = make(charm.Settings)
	if len(charmYamlConfig) > 0 {
		settings, err = ch.Config().ParseSettingsYAML([]byte(charmYamlConfig), args.ApplicationName)
		if err != nil {
			return errors.Trace(err)
		}
	}
	// Overlay any settings in YAML with those from config map.
	if len(charmConfig) > 0 {
		// Parse config in a compatible way (see function comment).
		overrideSettings, err := parseSettingsCompatible(ch.Config(), charmConfig)
		if err != nil {
			return errors.Trace(err)
		}
		for k, v := range overrideSettings {
			settings[k] = v
		}
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

	_, err = deployApplicationFunc(backend, DeployApplicationParams{
		ApplicationName:   args.ApplicationName,
		Series:            args.Series,
		Charm:             stateCharm(ch),
		Channel:           csparams.Channel(args.Channel),
		NumUnits:          args.NumUnits,
		ApplicationConfig: applicationConfig,
		CharmConfig:       settings,
		Constraints:       args.Constraints,
		Placement:         args.Placement,
		Storage:           args.Storage,
		Devices:           args.Devices,
		AttachStorage:     attachStorage,
		EndpointBindings:  args.EndpointBindings,
		Resources:         args.Resources,
	})
	return errors.Trace(err)
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

// applicationSetSettingsStrings updates the settings for the given application,
// taking the configuration from a map of strings.
func applicationSetSettingsStrings(
	application Application, gen string, settings map[string]string,
) error {
	ch, _, err := application.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	// Parse config in a compatible way (see function comment).
	changes, err := parseSettingsCompatible(ch.Config(), settings)
	if err != nil {
		return errors.Trace(err)
	}
	return application.UpdateCharmConfig(gen, changes)
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
	Channel               csparams.Channel
	ConfigSettingsStrings map[string]string
	ConfigSettingsYAML    string
	ResourceIDs           map[string]string
	StorageConstraints    map[string]params.StorageConstraints
	Force                 forceParams
}

type forceParams struct {
	ForceSeries, ForceUnits, Force bool
}

// Update updates the application attributes, including charm URL,
// minimum number of units, charm config and constraints.
// All parameters in params.ApplicationUpdate except the application name are optional.
func (api *APIBase) Update(args params.ApplicationUpdate) error {
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
		// For now we do not support changing the channel through Update().
		// TODO(ericsnow) Support it?
		channel := app.Channel()
		if err = api.updateCharm(
			setCharmParams{
				AppName:     args.ApplicationName,
				Application: app,
				Channel:     channel,
				Force: forceParams{
					ForceSeries: args.ForceSeries,
					ForceUnits:  args.ForceCharmURL,
					Force:       args.Force,
				},
			},
			args.CharmURL,
		); err != nil {
			return errors.Trace(err)
		}
	}
	// Set the minimum number of units for the given application.
	if args.MinUnits != nil {
		if err = app.SetMinUnits(*args.MinUnits); err != nil {
			return errors.Trace(err)
		}
	}

	// We need a guard on the API server-side for direct API callers such as
	// python-libjuju, and for older clients.
	// Always default to the master branch.
	if args.Generation == "" {
		args.Generation = model.GenerationMaster
	}

	// Set up application's settings.
	// If the config change is generational, add the app to the generation.
	configChange := false
	if args.SettingsYAML != "" {
		err = applicationSetCharmConfigYAML(args.ApplicationName, app, args.Generation, args.SettingsYAML)
		if err != nil {
			return errors.Annotate(err, "setting configuration from YAML")
		}
		configChange = true
	} else if len(args.SettingsStrings) > 0 {
		if err = applicationSetSettingsStrings(app, args.Generation, args.SettingsStrings); err != nil {
			return errors.Trace(err)
		}
		configChange = true
	}
	if configChange && args.Generation != model.GenerationMaster {
		if err := api.addAppToBranch(args.Generation, args.ApplicationName); err != nil {
			return errors.Trace(err)
		}
	}

	// Update application's constraints.
	if args.Constraints != nil {
		return app.SetConstraints(*args.Constraints)
	}
	return nil
}

// updateCharm parses the charm url and then grabs the charm from the backend.
// this is analogous to setCharmWithAgentValidation, minus the validation around
// setting the profile charm.
func (api *APIBase) updateCharm(
	params setCharmParams,
	url string,
) error {
	curl, err := charm.ParseURL(url)
	if err != nil {
		return errors.Trace(err)
	}
	aCharm, err := api.backend.Charm(curl)
	if err != nil {
		return errors.Trace(err)
	}
	return api.applicationSetCharm(params, aCharm)
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
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (api *APIBase) updateOneApplicationSeries(arg params.UpdateSeriesArg) error {
	if arg.Series == "" {
		return &params.Error{
			Message: "series missing from args",
			Code:    params.CodeBadRequest,
		}
	}
	applicationTag, err := names.ParseApplicationTag(arg.Entity.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	app, err := api.backend.Application(applicationTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	if !app.IsPrincipal() {
		return &params.Error{
			Message: fmt.Sprintf("%q is a subordinate application, update-series not supported", applicationTag.Id()),
			Code:    params.CodeNotSupported,
		}
	}
	if arg.Series == app.Series() {
		return nil // no-op
	}
	return app.UpdateApplicationSeries(arg.Series, arg.Force)
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
			Channel:               channel,
			ConfigSettingsStrings: args.ConfigSettings,
			ConfigSettingsYAML:    args.ConfigSettingsYAML,
			ResourceIDs:           args.ResourceIDs,
			StorageConstraints:    args.StorageConstraints,
			Force: forceParams{
				ForceSeries: args.ForceSeries,
				ForceUnits:  args.ForceUnits,
				Force:       args.Force,
			},
		},
		args.CharmURL,
	)
}

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
	if api.modelType == state.ModelTypeCAAS {
		return api.applicationSetCharm(params, newCharm)
	}

	oneApplication := params.Application
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
	currentCharm, _, err := oneApplication.Charm()
	if err != nil {
		logger.Debugf("Unable to locate current charm: %v", err)
	}
	if lxdprofile.NotEmpty(lxdCharmProfiler{Charm: currentCharm}) ||
		lxdprofile.NotEmpty(lxdCharmProfiler{Charm: newCharm}) {
		if err := validateAgentVersions(oneApplication, api.model); err != nil {
			return errors.Trace(err)
		}
	}

	return api.applicationSetCharm(params, newCharm)
}

// applicationSetCharm sets the charm for the given for the application.
func (api *APIBase) applicationSetCharm(
	params setCharmParams,
	stateCharm Charm,
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
		Channel:            params.Channel,
		ConfigSettings:     settings,
		ForceSeries:        force.ForceSeries,
		ForceUnits:         force.ForceUnits,
		Force:              force.Force,
		ResourceIDs:        params.ResourceIDs,
		StorageConstraints: stateStorageConstraints,
	}
	return params.Application.SetCharm(cfg)
}

// charmConfigFromGetYaml will parse a yaml produced by juju get and generate
// charm.Settings from it that can then be sent to the application.
func charmConfigFromGetYaml(yamlContents map[string]interface{}) (charm.Settings, error) {
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

// applicationSetCharmConfigYAML updates the charm config for the
// given application, taking the configuration from a YAML string.
func applicationSetCharmConfigYAML(
	appName string, application Application, gen string, settings string,
) error {
	b := []byte(settings)
	var all map[string]interface{}
	if err := goyaml.Unmarshal(b, &all); err != nil {
		return errors.Annotate(err, "parsing settings data")
	}
	// The file is already in the right format.
	if _, ok := all[appName]; !ok {
		changes, err := charmConfigFromGetYaml(all)
		if err != nil {
			return errors.Annotate(err, "processing YAML generated by get")
		}
		return errors.Annotate(application.UpdateCharmConfig(gen, changes), "updating settings with application YAML")
	}

	ch, _, err := application.Charm()
	if err != nil {
		return errors.Annotate(err, "obtaining charm for this application")
	}

	changes, err := ch.Config().ParseSettingsYAML(b, appName)
	if err != nil {
		return errors.Annotate(err, "creating config from YAML")
	}
	return errors.Annotate(application.UpdateCharmConfig(gen, changes), "updating settings")
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
				"cannot expose a CAAS application without a %q value set, run\n"+
					"juju config %s %s=<value>", caas.JujuExternalHostNameKey, args.ApplicationName, caas.JujuExternalHostNameKey)
		}
	}
	return app.SetExposed()
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
	return app.ClearExposed()
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
		return params.AddApplicationUnitsResults{}, errors.NotSupportedf("adding units on a non-container model")
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
	return common.DestroyErr("units", args.UnitNames, errs)
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
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Info = info
	}
	return params.DestroyUnitResults{results}, nil
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
		return common.ServerError(err)
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
				params.Entity{unit.UnitTag().String()},
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
						params.Entity{s.StorageTag().String()},
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
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Info = info
	}
	return params.DestroyApplicationResults{results}, nil
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
			results[i].Error = common.ServerError(err)
			continue
		}
		app, err := api.backend.RemoteApplication(appTag.Id())
		if err != nil {
			results[i].Error = common.ServerError(err)
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
			results[i].Error = common.ServerError(err)
			continue
		}
	}
	return params.ErrorResults{results}, nil
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
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Info = info
	}
	return params.ScaleApplicationResults{results}, nil
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
		results.Results[i].Error = common.ServerError(err)
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
		results[i].Error = common.ServerError(err)
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
		results[i].Error = common.ServerError(err)
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
			Name:       space.Name,
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
	return params.GetConstraintsResults{cons}, errors.Trace(err)
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
		results.Results[i].Error = common.ServerError(err)
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
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if tag.Kind() != names.ApplicationTagKind {
			results.Results[i].Error = common.ServerError(
				errors.Errorf("unexpected tag type, expected application, got %s", tag.Kind()))
			continue
		}

		// Always deal with the master branch version of config.
		config, err := api.getCharmConfig(model.GenerationMaster, tag.Id())
		results.Results[i].Config = config
		results.Results[i].Error = common.ServerError(err)
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
func (api *APIBase) SetApplicationsConfig(args params.ApplicationConfigSetArgs) (params.ErrorResults, error) {
	var result params.ErrorResults
	if err := api.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return result, errors.Trace(err)
	}
	result.Results = make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		err := api.setApplicationConfig(arg)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (api *APIBase) setApplicationConfig(arg params.ApplicationConfigSet) error {
	app, err := api.backend.Application(arg.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}

	appConfigAttrs, charmConfig, err := splitApplicationAndCharmConfig(api.modelType, arg.Config)
	if err != nil {
		return errors.Trace(err)
	}
	configSchema, defaults, err := applicationConfigSchema(api.modelType)
	if err != nil {
		return errors.Trace(err)
	}

	if len(appConfigAttrs) > 0 {
		if err := app.UpdateApplicationConfig(appConfigAttrs, nil, configSchema, defaults); err != nil {
			return errors.Annotate(err, "updating application config values")
		}
	}
	if len(charmConfig) > 0 {
		ch, _, err := app.Charm()
		if err != nil {
			return errors.Trace(err)
		}
		// Validate the charm and application config.
		charmConfigChanges, err := ch.Config().ParseSettingsStrings(charmConfig)
		if err != nil {
			return errors.Trace(err)
		}

		// We need a guard on the API server-side for direct API callers such as
		// python-libjuju, and for older clients.
		// Always default to the master branch.
		if arg.Generation == "" {
			arg.Generation = model.GenerationMaster
		}
		if err := app.UpdateCharmConfig(arg.Generation, charmConfigChanges); err != nil {
			return errors.Annotate(err, "updating application charm settings")
		}
		if arg.Generation != model.GenerationMaster {
			if err := api.addAppToBranch(arg.Generation, arg.ApplicationName); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
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
		result.Results[i].Error = common.ServerError(err)
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
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		unit, err := api.backend.Unit(tag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = unit.Resolve(p.Retry)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ApplicationInfo isn't on the v8 API.
func (u *APIv8) ApplicationInfo(_, _ struct{}) {}

// ApplicationsInfo returns applications information.
func (api *APIBase) ApplicationsInfo(in params.Entities) (params.ApplicationInfoResults, error) {
	out := make([]params.ApplicationInfoResult, len(in.Entities))
	for i, one := range in.Entities {
		tag, err := names.ParseApplicationTag(one.Tag)
		if err != nil {
			out[i].Error = common.ServerError(err)
			continue
		}
		app, err := api.backend.Application(tag.Name)
		if err != nil {
			out[i].Error = common.ServerError(err)
			continue
		}

		details, err := api.getConfig(params.ApplicationGet{ApplicationName: tag.Name}, describe)
		if err != nil {
			out[i].Error = common.ServerError(err)
			continue
		}

		bindings, err := app.EndpointBindings()
		if err != nil {
			out[i].Error = common.ServerError(err)
			continue
		}

		out[i].Result = &params.ApplicationInfo{
			Tag:              tag.String(),
			Charm:            details.Charm,
			Series:           details.Series,
			Channel:          details.Channel,
			Constraints:      details.Constraints,
			Principal:        app.IsPrincipal(),
			Exposed:          app.IsExposed(),
			Remote:           app.IsRemote(),
			EndpointBindings: bindings,
		}
	}
	return params.ApplicationInfoResults{out}, nil
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
