// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"math"
	"net"
	"reflect"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/schema"
	"github.com/juju/version/v2"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/macaroon.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/caasoperatorprovisioner"
	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes/provider"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/charmhub"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/bootstrap"
	environsconfig "github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	jujusecrets "github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

var ClassifyDetachedStorage = storagecommon.ClassifyDetachedStorage

var logger = loggo.GetLogger("juju.apiserver.application")

// APIv20 provides the Application API facade for version 20.
type APIv20 struct {
	*APIBase
}

// APIv19 provides the Application API facade for version 19.
type APIv19 struct {
	*APIv20
}

// APIv18 provides the Application API facade for version 18.
type APIv18 struct {
	*APIv19
}

// APIv17 provides the Application API facade for version 17.
type APIv17 struct {
	*APIv18
}

// APIv16 provides the Application API facade for version 16.
type APIv16 struct {
	*APIv17
}

// APIv15 provides the Application API facade for version 15.
type APIv15 struct {
	*APIv16
}

// APIBase implements the shared application interface and is the concrete
// implementation of the api end point.
type APIBase struct {
	backend       Backend
	storageAccess StorageInterface

	authorizer facade.Authorizer
	check      BlockChecker
	updateBase UpdateBase
	repoDeploy DeployFromRepository

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
	caasBroker            CaasBrokerInterface
	deployApplicationFunc func(ApplicationDeployer, Model, DeployApplicationParams) (Application, error)

	secretBackendConfigGetter jujusecrets.BackendConfigForDeleteGetter
	secretsState              SecretsState
}

type CaasBrokerInterface interface {
	ValidateStorageClass(config map[string]interface{}) error
	Version() (*version.Number, error)
}

func newFacadeBase(ctx facade.Context) (*APIBase, error) {
	m, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Annotate(err, "getting model")
	}
	// modelShim wraps the AllPorts() API.
	model := &modelShim{Model: m}
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
	modelType := model.Type()
	if modelType == state.ModelTypeCAAS {
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

	st := &stateShim{ctx.State()}

	modelCfg, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}

	charmhubHTTPClient := ctx.HTTPClient(facade.CharmhubHTTPClient)
	chURL, _ := modelCfg.CharmHubURL()
	chClient, err := charmhub.NewClient(charmhub.Config{
		URL:        chURL,
		HTTPClient: charmhubHTTPClient,
		Logger:     logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	updateBase := NewUpdateBaseAPI(st, makeUpdateSeriesValidator(chClient))
	validatorCfg := validatorConfig{
		charmhubHTTPClient: charmhubHTTPClient,
		caasBroker:         caasBroker,
		model:              m,
		registry:           registry,
		state:              st,
		storagePoolManager: storagePoolManager,
	}
	repoDeploy := NewDeployFromRepositoryAPI(st, makeDeployFromRepositoryValidator(validatorCfg))

	secretBackendConfigGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		return commonsecrets.SecretCleanupBackendConfigInfo(commonsecrets.SecretsModel(m), backendID)
	}

	return NewAPIBase(
		st,
		storageAccess,
		ctx.Auth(),
		updateBase,
		repoDeploy,
		blockChecker,
		model,
		leadershipReader,
		stateCharm,
		DeployApplication,
		storagePoolManager,
		registry,
		resources,
		caasBroker,
		secretBackendConfigGetter,
		state.NewSecrets(ctx.State()),
	)
}

// NewAPIBase returns a new application API facade.
func NewAPIBase(
	backend Backend,
	storageAccess StorageInterface,
	authorizer facade.Authorizer,
	updateBase UpdateBase,
	repoDeploy DeployFromRepository,
	blockChecker BlockChecker,
	model Model,
	leadershipReader leadership.Reader,
	stateCharm func(Charm) *state.Charm,
	deployApplication func(ApplicationDeployer, Model, DeployApplicationParams) (Application, error),
	storagePoolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
	resources facade.Resources,
	caasBroker CaasBrokerInterface,
	secretBackendConfigGetter jujusecrets.BackendConfigForDeleteGetter,
	secretsState SecretsState,
) (*APIBase, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	return &APIBase{
		backend:                   backend,
		storageAccess:             storageAccess,
		authorizer:                authorizer,
		updateBase:                updateBase,
		repoDeploy:                repoDeploy,
		check:                     blockChecker,
		model:                     model,
		modelType:                 model.Type(),
		leadershipReader:          leadershipReader,
		stateCharm:                stateCharm,
		deployApplicationFunc:     deployApplication,
		storagePoolManager:        storagePoolManager,
		registry:                  registry,
		resources:                 resources,
		caasBroker:                caasBroker,
		secretBackendConfigGetter: secretBackendConfigGetter,
		secretsState:              secretsState,
	}, nil
}

func (api *APIBase) checkCanRead() error {
	return api.authorizer.HasPermission(permission.ReadAccess, api.model.ModelTag())
}

func (api *APIBase) checkCanWrite() error {
	return api.authorizer.HasPermission(permission.WriteAccess, api.model.ModelTag())
}

// SetMetricCredentials sets credentials on the application.
// TODO (cderici) only used for metered charms in cmd MeteredDeployAPI,
// kept for client compatibility, remove in juju 4.0
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
		if err := common.ValidateCharmOrigin(arg.CharmOrigin); err != nil {
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
			resources := api.backend.Resources()
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
	appConfigKeys := config.KnownConfigKeys(providerSchema)

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
	appConfigKeys := config.KnownConfigKeys(providerSchema)

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

// caasDeployParams contains deploy configuration requiring prechecks
// specific to a caas.
type caasDeployParams struct {
	applicationName string
	attachStorage   []string
	charm           CharmMeta
	config          map[string]string
	placement       []*instance.Placement
	storage         map[string]storage.Constraints
}

// precheck, checks the deploy config based on caas specific
// requirements.
func (c caasDeployParams) precheck(
	model Model,
	storagePoolManager poolmanager.PoolManager,
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
	serviceType := c.config[k8s.ServiceTypeConfigKey]
	if _, err := k8s.CaasServiceToK8s(caas.ServiceType(serviceType)); err != nil {
		return errors.NotValidf("service type %q", serviceType)
	}

	cfg, err := model.ModelConfig()
	if err != nil {
		return errors.Trace(err)
	}

	// For older charms, operator-storage model config is mandatory.
	if k8s.RequireOperatorStorage(c.charm) {
		storageClassName, _ := cfg.AllAttrs()[k8sconstants.OperatorStorageKey].(string)
		if storageClassName == "" {
			return errors.New(
				"deploying this Kubernetes application requires a suitable storage class.\n" +
					"None have been configured. Set the operator-storage model config to " +
					"specify which storage class should be used to allocate operator storage.\n" +
					"See https://discourse.charmhub.io/t/getting-started/152.",
			)
		}
		sp, err := caasoperatorprovisioner.CharmStorageParams("", storageClassName, cfg, "", storagePoolManager, registry)
		if err != nil {
			return errors.Annotatef(err, "getting operator storage params for %q", c.applicationName)
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
	for storageName, cons := range c.storage {
		if cons.Pool == "" && workloadStorageClass == "" {
			return errors.Errorf("storage pool for %q must be specified since there's no model default storage class", storageName)
		}
		_, err := caasoperatorprovisioner.CharmStorageParams("", workloadStorageClass, cfg, cons.Pool, storagePoolManager, registry)
		if err != nil {
			return errors.Annotatef(err, "getting workload storage params for %q", c.applicationName)
		}
	}

	caasVersion, err := caasBroker.Version()
	if err != nil {
		return errors.Trace(err)
	}
	if err := checkCAASMinVersion(c.charm, caasVersion); err != nil {
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
	deployApplicationFunc func(ApplicationDeployer, Model, DeployApplicationParams) (Application, error),
	storagePoolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
	caasBroker CaasBrokerInterface,
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
	if err := checkMachinePlacement(backend, model.UUID(), args.ApplicationName, args.Placement); err != nil {
		return errors.Trace(err)
	}

	// Try to find the charm URL in state first.
	ch, err := backend.Charm(args.CharmURL)
	if err != nil {
		return errors.Trace(err)
	}

	if err := jujuversion.CheckJujuMinVersion(ch.Meta().MinJujuVersion, jujuversion.Current); err != nil {
		return errors.Trace(err)
	}

	modelType := model.Type()
	if modelType != state.ModelTypeIAAS {
		caas := caasDeployParams{
			applicationName: args.ApplicationName,
			attachStorage:   args.AttachStorage,
			charm:           ch,
			config:          args.Config,
			placement:       args.Placement,
			storage:         args.Storage,
		}
		if err := caas.precheck(model, storagePoolManager, registry, caasBroker); err != nil {
			return errors.Trace(err)
		}
	}

	appConfig, _, charmSettings, _, err := parseCharmSettings(modelType, ch.Config(), args.ApplicationName, args.Config, args.ConfigYAML, environsconfig.UseDefaults)
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
	origin, err := convertCharmOrigin(args.CharmOrigin)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = deployApplicationFunc(backend, model, DeployApplicationParams{
		ApplicationName:   args.ApplicationName,
		Charm:             stateCharm(ch),
		CharmOrigin:       origin,
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
		Force:             args.Force,
	})
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

func validateSecretConfig(chCfg *charm.Config, cfg charm.Settings) error {
	for name, value := range cfg {
		option, ok := chCfg.Options[name]
		if !ok {
			// This should never happen.
			return errors.NotValidf("unknown option %q", name)
		}
		if option.Type == "secret" {
			uriStr, ok := value.(string)
			if !ok {
				return errors.NotValidf("secret value should be a string, got %T instead", name)
			}
			if uriStr == "" {
				return nil
			}
			_, err := secrets.ParseURI(uriStr)
			return errors.Annotatef(err, "invalid secret URI for option %q", name)
		}
	}
	return nil
}

// parseCharmSettings parses, verifies and combines the config settings for a
// charm as specified by the provided config map and config yaml payload. Any
// model-specific application settings will be automatically extracted and
// returned back as an *application.Config.
func parseCharmSettings(
	modelType state.ModelType, chCfg *charm.Config, appName string,
	cfg map[string]string, configYaml string, defaults environsconfig.Defaulting,
) (_ *config.Config, _ environschema.Fields, chOut charm.Settings, _ schema.Defaults, err error) {
	defer func() {
		if chOut != nil {
			err = validateSecretConfig(chCfg, chOut)
		}
	}()

	// Split out the app config from the charm config for any config
	// passed in as a map as opposed to YAML.
	var (
		applicationConfig map[string]interface{}
		charmConfig       map[string]string
	)
	if len(cfg) > 0 {
		if applicationConfig, charmConfig, err = splitApplicationAndCharmConfig(modelType, cfg); err != nil {
			return nil, nil, nil, nil, errors.Trace(err)
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
			return nil, nil, nil, nil, errors.Trace(err)
		}
	}

	// Entries from the string-based config map always override any entries
	// provided via the YAML payload.
	for k, v := range applicationConfig {
		appSettings[k] = v
	}

	appCfgSchema, schemaDefaults, err := applicationConfigSchema(modelType)
	if err != nil {
		return nil, nil, nil, nil, errors.Trace(err)
	}
	getDefaults := func() schema.Defaults {
		// If defaults is UseDefaults, defaults are baked into the app config.
		if defaults == environsconfig.UseDefaults {
			return schemaDefaults
		}
		return nil
	}
	appConfig, err := config.NewConfig(appSettings, appCfgSchema, getDefaults())
	if err != nil {
		return nil, nil, nil, nil, errors.Trace(err)
	}

	// If there isn't a charm YAML, then we can just return the charmConfig as
	// the settings and no need to attempt to parse an empty yaml.
	if len(charmYamlConfig) == 0 {
		settings, err := chCfg.ParseSettingsStrings(charmConfig)
		if err != nil {
			return nil, nil, nil, nil, errors.Trace(err)
		}
		return appConfig, appCfgSchema, settings, schemaDefaults, nil
	}

	var charmSettings charm.Settings
	// Parse the charm YAML and check the yaml against the charm config.
	if charmSettings, err = chCfg.ParseSettingsYAML([]byte(charmYamlConfig), appName); err != nil {
		// Check if this is 'juju get' output and parse it as such
		jujuGetSettings, pErr := charmConfigFromYamlConfigValues(charmYamlConfig)
		if pErr != nil {
			// Not 'juju output' either; return original error
			return nil, nil, nil, nil, errors.Trace(err)
		}
		charmSettings = jujuGetSettings
	}

	// Entries from the string-based config map always override any entries
	// provided via the YAML payload.
	if len(charmConfig) != 0 {
		// Parse config in a compatible way (see function comment).
		overrideSettings, err := parseSettingsCompatible(chCfg, charmConfig)
		if err != nil {
			return nil, nil, nil, nil, errors.Trace(err)
		}
		for k, v := range overrideSettings {
			charmSettings[k] = v
		}
	}

	return appConfig, appCfgSchema, charmSettings, schemaDefaults, nil
}

type MachinePlacementBackend interface {
	Machine(string) (Machine, error)
}

// checkMachinePlacement does a non-exhaustive validation of any supplied
// placement directives.
// If the placement scope is for a machine, ensure that the machine exists.
// If the placement is for a machine or a container on an existing machine,
// check that the machine is not locked for series upgrade.
// If the placement scope is model-uuid, replace it with the actual model uuid.
func checkMachinePlacement(backend MachinePlacementBackend, modelUUID string, app string, placement []*instance.Placement) error {
	errTemplate := "cannot deploy %q to machine %s"

	for _, p := range placement {
		if p == nil {
			continue
		}
		// Substitute the placeholder with the actual model uuid.
		if p.Scope == "model-uuid" {
			p.Scope = modelUUID
			continue
		}

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
	ConfigSettingsStrings map[string]string
	ConfigSettingsYAML    string
	ResourceIDs           map[string]string
	StorageConstraints    map[string]params.StorageConstraints
	EndpointBindings      map[string]string
	Force                 forceParams
}

type forceParams struct {
	ForceBase, ForceUnits, Force bool
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

	// parseCharmSettings is passed false for useDefaults because setConfig
	// should not care about defaults.
	// If defaults are wanted, one should call unsetApplicationConfig.
	appConfig, appConfigSchema, charmSettings, defaults, err := parseCharmSettings(api.modelType, ch.Config(), app.Name(), settingsStrings, settingsYAML, environsconfig.NoDefaults)
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
		if err = app.UpdateApplicationConfig(cfgAttrs, nil, appConfigSchema, defaults); err != nil {
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

// UpdateApplicationBase updates the application base.
// Base for subordinates is updated too.
func (api *APIBase) UpdateApplicationBase(args params.UpdateChannelArgs) (params.ErrorResults, error) {
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
		err := api.updateOneApplicationBase(arg)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (api *APIBase) updateOneApplicationBase(arg params.UpdateChannelArg) error {
	var argBase corebase.Base
	if arg.Channel != "" {
		appTag, err := names.ParseTag(arg.Entity.Tag)
		if err != nil {
			return errors.Trace(err)
		}
		app, err := api.backend.Application(appTag.Id())
		if err != nil {
			return errors.Trace(err)
		}
		origin := app.CharmOrigin()
		argBase, err = corebase.ParseBase(origin.Platform.OS, arg.Channel)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return api.updateBase.UpdateBase(arg.Entity.Tag, argBase, arg.Force)
}

// SetCharm sets the charm for a given for the application.
func (api *APIBase) SetCharm(args params.ApplicationSetCharm) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}

	if err := common.ValidateCharmOrigin(args.CharmOrigin); err != nil {
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
	return api.setCharmWithAgentValidation(
		setCharmParams{
			AppName:               args.ApplicationName,
			Application:           oneApplication,
			CharmOrigin:           args.CharmOrigin,
			ConfigSettingsStrings: args.ConfigSettings,
			ConfigSettingsYAML:    args.ConfigSettingsYAML,
			ResourceIDs:           args.ResourceIDs,
			StorageConstraints:    args.StorageConstraints,
			EndpointBindings:      args.EndpointBindings,
			Force: forceParams{
				ForceBase:  args.ForceBase,
				ForceUnits: args.ForceUnits,
				Force:      args.Force,
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
	newCharm, err := api.backend.Charm(url)
	if err != nil {
		return errors.Trace(err)
	}
	oneApplication := params.Application
	currentCharm, _, err := oneApplication.Charm()
	if err != nil {
		logger.Debugf("Unable to locate current charm: %v", err)
	}
	newOrigin, err := convertCharmOrigin(params.CharmOrigin)
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
		origin, err := StateCharmOrigin(newOrigin)
		if err != nil {
			return errors.Trace(err)
		}
		return api.applicationSetCharm(params, newCharm, origin)
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

	origin, err := StateCharmOrigin(newOrigin)
	if err != nil {
		return errors.Trace(err)
	}
	return api.applicationSetCharm(params, newCharm, origin)
}

// applicationSetCharm sets the charm and updated config
// for the given application.
func (api *APIBase) applicationSetCharm(
	params setCharmParams,
	newCharm Charm,
	newOrigin *state.CharmOrigin,
) error {
	model, err := api.backend.Model()
	if err != nil {
		return errors.Annotate(err, "retrieving model")
	}
	modelType := model.Type()

	appConfig, appSchema, charmSettings, appDefaults, err := parseCharmSettings(modelType, newCharm.Config(), params.AppName, params.ConfigSettingsStrings, params.ConfigSettingsYAML, environsconfig.NoDefaults)
	if err != nil {
		return errors.Annotate(err, "parsing config settings")
	}
	if err := appConfig.Validate(); err != nil {
		return errors.Annotate(err, "validating config settings")
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

	// Enforce "assumes" requirements if the feature flag is enabled.
	if err := assertCharmAssumptions(newCharm.Meta().Assumes, model, api.backend.ControllerConfig); err != nil {
		if !errors.IsNotSupported(err) || !params.Force.Force {
			return errors.Trace(err)
		}

		logger.Warningf("proceeding with upgrade of application %q even though the charm feature requirements could not be met as --force was specified", params.AppName)
	}

	force := params.Force
	cfg := state.SetCharmConfig{
		Charm:              api.stateCharm(newCharm),
		CharmOrigin:        newOrigin,
		ForceBase:          force.ForceBase,
		ForceUnits:         force.ForceUnits,
		Force:              force.Force,
		PendingResourceIDs: params.ResourceIDs,
		StorageConstraints: stateStorageConstraints,
		EndpointBindings:   params.EndpointBindings,
	}
	if len(charmSettings) > 0 {
		cfg.ConfigSettings = charmSettings
	}

	// Disallow downgrading from a v2 charm to a v1 charm.
	oldCharm, _, err := params.Application.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	if charm.MetaFormat(oldCharm) >= charm.FormatV2 && charm.MetaFormat(newCharm) == charm.FormatV1 {
		return errors.New("cannot downgrade from v2 charm format to v1")
	}

	// If upgrading from a pod-spec (v1) charm to sidecar (v2), force the application
	// to have no units.
	if charm.MetaFormat(oldCharm) == charm.FormatV1 && corecharm.IsKubernetes(oldCharm) &&
		charm.MetaFormat(newCharm) >= charm.FormatV2 && corecharm.IsKubernetes(newCharm) {
		cfg.RequireNoUnits = true
	}

	// TODO(wallyworld) - do in a single transaction
	if err := params.Application.SetCharm(cfg); err != nil {
		return errors.Annotate(err, "updating charm config")
	}
	if attr := appConfig.Attributes(); len(attr) > 0 {
		return params.Application.UpdateApplicationConfig(attr, nil, appSchema, appDefaults)
	}
	return nil
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

	for setting, content := range settingsMap {
		s, ok := content.(map[interface{}]interface{})
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
	result := params.CharmURLOriginResult{URL: *charmURL}
	chOrigin := oneApplication.CharmOrigin()
	if chOrigin == nil {
		result.Error = apiservererrors.ServerError(errors.NotFoundf("charm origin for %q", args.ApplicationName))
		return result, nil
	}
	if result.Origin, err = makeParamsCharmOrigin(chOrigin); err != nil {
		result.Error = apiservererrors.ServerError(errors.NotFoundf("charm origin for %q", args.ApplicationName))
		return result, nil
	}
	result.Origin.InstanceKey = charmhub.CreateInstanceKey(oneApplication.ApplicationTag(), api.model.ModelTag())
	return result, nil
}

func makeParamsCharmOrigin(origin *state.CharmOrigin) (params.CharmOrigin, error) {
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
		if origin.Channel.Branch != "" {
			retOrigin.Branch = &origin.Channel.Branch
		}
	}
	if origin.Platform != nil {
		retOrigin.Architecture = origin.Platform.Architecture
		retOrigin.Base = params.Base{Name: origin.Platform.OS, Channel: origin.Platform.Channel}
	}
	return retOrigin, nil
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
func (api *APIBase) AddUnits(args params.AddApplicationUnits) (params.AddApplicationUnitsResults, error) {
	if api.modelType == state.ModelTypeCAAS {
		return params.AddApplicationUnitsResults{}, errors.NotSupportedf("adding units to a container-based model")
	}

	// TODO(wallyworld) - enable-ha is how we add new controllers at the moment
	// Remove this check before 3.0 when enable-ha is refactored.
	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	ch, _, err := app.Charm()
	if err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	if ch.Meta().Name == bootstrap.ControllerCharmName {
		return params.AddApplicationUnitsResults{}, errors.NotSupportedf("adding units to the controller application")
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
// TODO(jack-w-shaw) Drop this once facade 16 is not longer supported
func (api *APIv16) DestroyUnits(args params.DestroyApplicationUnits) error {
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

func (*APIBase) DestroyUnits(_, _ struct{}) {}

func (api *APIv15) DestroyUnit(argsV15 params.DestroyUnitsParamsV15) (params.DestroyUnitResults, error) {
	args := params.DestroyUnitsParams{
		Units: transform.Slice(argsV15.Units, func(p params.DestroyUnitParamsV15) params.DestroyUnitParams {
			return params.DestroyUnitParams{
				UnitTag:        p.UnitTag,
				DestroyStorage: p.DestroyStorage,
				Force:          p.Force,
				MaxWait:        p.MaxWait,
				DryRun:         false,
			}
		}),
	}
	return api.APIv16.DestroyUnit(args)
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

	appCharms := make(map[string]Charm)
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
			return nil, errors.Errorf("unit %q is a subordinate, to remove use remove-relation. Note: this will remove all units of %q", name, unit.ApplicationName())
		}

		// TODO(wallyworld) - enable-ha is how we remove controllers at the moment
		// Remove this check before 3.0 when enable-ha is refactored.
		appName, _ := names.UnitApplication(unitTag.Id())
		ch, ok := appCharms[appName]
		if !ok {
			app, err := api.backend.Application(appName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ch, _, err = app.Charm()
			if err != nil {
				return nil, errors.Trace(err)
			}
			appCharms[appName] = ch
		}
		if ch.Meta().Name == bootstrap.ControllerCharmName {
			return nil, errors.NotSupportedf("removing units from the controller application")
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
			info.DestroyedStorage, info.DetachedStorage, err = ClassifyDetachedStorage(
				api.storageAccess.VolumeAccess(), api.storageAccess.FilesystemAccess(), unitStorage,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		if arg.DryRun {
			return &info, nil
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
// TODO(jack-w-shaw) Drop this once facade 16 is not longer supported
func (api *APIv16) Destroy(in params.ApplicationDestroy) error {
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

func (*APIBase) Destroy(_, _ struct{}) {}

// DestroyApplication removes a given set of applications.
func (api *APIv15) DestroyApplication(argsV15 params.DestroyApplicationsParamsV15) (params.DestroyApplicationResults, error) {
	args := params.DestroyApplicationsParams{
		Applications: transform.Slice(argsV15.Applications, func(p params.DestroyApplicationParamsV15) params.DestroyApplicationParams {
			return params.DestroyApplicationParams{
				ApplicationTag: p.ApplicationTag,
				DestroyStorage: p.DestroyStorage,
				Force:          p.Force,
				MaxWait:        p.MaxWait,
				DryRun:         false,
			}
		}),
	}
	return api.APIBase.DestroyApplication(args)
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

		ch, _, err := app.Charm()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if ch.Meta().Name == bootstrap.ControllerCharmName {
			return nil, errors.NotSupportedf("removing the controller application")
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
				destroyed, detached, err := ClassifyDetachedStorage(
					api.storageAccess.VolumeAccess(), api.storageAccess.FilesystemAccess(), unitStorage,
				)
				if err != nil {
					return nil, err
				}
				info.DestroyedStorage = append(info.DestroyedStorage, destroyed...)
				info.DetachedStorage = append(info.DetachedStorage, detached...)
			}
		}

		if arg.DryRun {
			return &info, nil
		}

		op := app.DestroyOperation()
		op.DestroyStorage = arg.DestroyStorage
		op.Force = arg.Force
		if arg.Force {
			op.MaxWait = common.MaxWait(arg.MaxWait)
		}
		backend := jujusecrets.NewClientForContentDeletion(api.secretsState, api.secretBackendConfigGetter)
		op.SecretContentDeleter = backend.DeleteContent
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

	inEps, err := api.backend.InferEndpoints(args.Endpoints...)
	if err != nil {
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
	if len(args.ViaCIDRs) > 0 {
		var isCrossModel bool
		for _, ep := range inEps {
			_, err = api.backend.RemoteApplication(ep.ApplicationName)
			if err == nil {
				isCrossModel = true
				break
			} else if !errors.IsNotFound(err) {
				return params.AddRelationResults{}, errors.Trace(err)
			}
		}
		if !isCrossModel {
			return params.AddRelationResults{}, errors.NotSupportedf("integration via subnets for non cross model relations")
		}
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
	var rel Relation
	if len(args.Endpoints) > 0 {
		rel, err = api.backend.InferActiveRelation(args.Endpoints...)
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
		oc, err := api.backend.OfferConnectionForRelation(rel.Tag().Id())
		if errors.IsNotFound(err) {
			return errors.Errorf("cannot set suspend status for %q which is not associated with an offer", rel.Tag().Id())
		}
		if oc != nil && !arg.Suspended && rel.Suspended() {
			ok, err := commoncrossmodel.CheckCanConsume(api.authorizer, api.backend, api.backend.ControllerTag(), api.model.ModelTag(), oc)
			if err != nil {
				return errors.Trace(err)
			}
			if !ok {
				return apiservererrors.ErrPerm
			}
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
func (api *APIBase) Consume(args params.ConsumeApplicationArgsV5) (params.ErrorResults, error) {
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

func (api *APIBase) consumeOne(arg params.ConsumeApplicationArgV5) error {
	sourceModelTag, err := names.ParseModelTag(arg.SourceModelTag)
	if err != nil {
		return errors.Trace(err)
	}

	// Maybe save the details of the controller hosting the offer.
	var externalControllerUUID string
	if arg.ControllerInfo != nil {
		controllerTag, err := names.ParseControllerTag(arg.ControllerInfo.ControllerTag)
		if err != nil {
			return errors.Trace(err)
		}
		// Only save controller details if the offer comes from
		// a different controller.
		if controllerTag.Id() != api.backend.ControllerTag().Id() {
			externalControllerUUID = controllerTag.Id()
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
	_, err = api.saveRemoteApplication(sourceModelTag, appName, externalControllerUUID, arg.ApplicationOfferDetailsV5, arg.Macaroon)
	return err
}

// Consume adds remote applications to the model without creating any
// relations.
func (api *APIv19) Consume(args params.ConsumeApplicationArgsV4) (params.ErrorResults, error) {
	var consumeApplicationArgs []params.ConsumeApplicationArgV5
	for _, arg := range args.Args {
		consumeApplicationArgs = append(consumeApplicationArgs, params.ConsumeApplicationArgV5{
			Macaroon:                  arg.Macaroon,
			ControllerInfo:            arg.ControllerInfo,
			ApplicationAlias:          arg.ApplicationAlias,
			ApplicationOfferDetailsV5: arg.ApplicationOfferDetailsV5,
		})
	}
	return api.APIv20.Consume(params.ConsumeApplicationArgsV5{
		Args: consumeApplicationArgs,
	})
}

// saveRemoteApplication saves the details of the specified remote application and its endpoints
// to the state model so relations to the remote application can be created.
func (api *APIBase) saveRemoteApplication(
	sourceModelTag names.ModelTag,
	applicationName string,
	externalControllerUUID string,
	offer params.ApplicationOfferDetailsV5,
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

	// If a remote application with the same name and endpoints from the same
	// source model already exists, we will use that one.
	// If the status was "terminated", the offer had been removed, so we'll replace
	// the terminated application with a fresh copy.
	remoteApp, appStatus, err := api.maybeUpdateExistingApplicationEndpoints(applicationName, sourceModelTag, remoteEps)
	if err == nil {
		if appStatus != status.Terminated {
			return remoteApp, nil
		}
		// If the same application was previously terminated due to the offer being removed,
		// first ensure we delete it from this consuming model before adding again.
		// TODO(wallyworld) - this operation should be in a single txn.
		logger.Debugf("removing terminated remote app %q before adding a replacement", applicationName)
		op := remoteApp.DestroyOperation(true)
		if err := api.backend.ApplyOperation(op); err != nil {
			return nil, errors.Annotatef(err, "removing terminated saas application %q", applicationName)
		}
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	return api.backend.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:                   applicationName,
		OfferUUID:              offer.OfferUUID,
		URL:                    offer.OfferURL,
		ExternalControllerUUID: externalControllerUUID,
		SourceModel:            sourceModelTag,
		Endpoints:              remoteEps,
		Macaroon:               mac,
	})
}

// maybeUpdateExistingApplicationEndpoints looks for a remote application with the
// specified name and source model tag and tries to update its endpoints with the
// new ones specified. If the endpoints are compatible, the newly updated remote
// application is returned.
// If the application status is Terminated, no updates are done.
func (api *APIBase) maybeUpdateExistingApplicationEndpoints(
	applicationName string, sourceModelTag names.ModelTag, remoteEps []charm.Relation,
) (RemoteApplication, status.Status, error) {
	existingRemoteApp, err := api.backend.RemoteApplication(applicationName)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	if existingRemoteApp.SourceModel().Id() != sourceModelTag.Id() {
		return nil, "", errors.AlreadyExistsf("saas application called %q from a different model", applicationName)
	}
	if existingRemoteApp.Life() != state.Alive {
		return nil, "", errors.NewAlreadyExists(nil, fmt.Sprintf("saas application called %q exists but is terminating", applicationName))
	}
	appStatus, err := existingRemoteApp.Status()
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	if appStatus.Status == status.Terminated {
		return existingRemoteApp, appStatus.Status, nil
	}
	newEpsMap := make(map[charm.Relation]bool)
	for _, ep := range remoteEps {
		newEpsMap[ep] = true
	}
	existingEps, err := existingRemoteApp.Endpoints()
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	maybeSameEndpoints := len(newEpsMap) == len(existingEps)
	existingEpsByName := make(map[string]charm.Relation)
	for _, ep := range existingEps {
		existingEpsByName[ep.Name] = ep.Relation
		delete(newEpsMap, ep.Relation)
	}
	sameEndpoints := maybeSameEndpoints && len(newEpsMap) == 0
	if sameEndpoints {
		return existingRemoteApp, appStatus.Status, nil
	}

	// Gather the new endpoints. All new endpoints passed to AddEndpoints()
	// below must not have the same name as an existing endpoint.
	var newEps []charm.Relation
	for ep := range newEpsMap {
		// See if we are attempting to update endpoints with the same name but
		// different relation data.
		if existing, ok := existingEpsByName[ep.Name]; ok && existing != ep {
			return nil, "", errors.Errorf("conflicting endpoint %v", ep.Name)
		}
		newEps = append(newEps, ep)
	}

	if len(newEps) > 0 {
		// Update the existing remote app to have the new, additional endpoints.
		if err := existingRemoteApp.AddEndpoints(newEps); err != nil {
			return nil, "", errors.Trace(err)
		}
	}
	return existingRemoteApp, appStatus.Status, nil
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

// SetConfigs implements the server side of Application.SetConfig.  Both
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
	appConfigFields := config.KnownConfigKeys(configSchema)

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
			Base:             details.Base,
			Channel:          channel,
			Constraints:      details.Constraints,
			Principal:        app.IsPrincipal(),
			Exposed:          app.IsExposed(),
			Remote:           app.IsRemote(),
			Life:             app.Life().String(),
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
			"Please run juju upgrade-model to upgrade the current model to match your controller.")
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

// UnitsInfo returns unit information for the given entities (units or
// applications).
func (api *APIBase) UnitsInfo(in params.Entities) (params.UnitInfoResults, error) {
	var results []params.UnitInfoResult
	leaders, err := api.leadershipReader.Leaders()
	if err != nil {
		return params.UnitInfoResults{}, errors.Trace(err)
	}
	for _, one := range in.Entities {
		units, err := api.unitsFromTag(one.Tag)
		if err != nil {
			results = append(results, params.UnitInfoResult{Error: apiservererrors.ServerError(err)})
			continue
		}
		for _, unit := range units {
			result, err := api.unitResultForUnit(unit)
			if err != nil {
				results = append(results, params.UnitInfoResult{Error: apiservererrors.ServerError(err)})
				continue
			}
			if leader := leaders[unit.ApplicationName()]; leader == unit.Name() {
				result.Leader = true
			}
			results = append(results, params.UnitInfoResult{Result: result})
		}
	}
	return params.UnitInfoResults{
		Results: results,
	}, nil
}

// Returns the units referred to by the tag argument.  If the tag refers to a
// unit, a slice with a single unit is returned.  If the tag refers to an
// application, all the units in the application are returned.
func (api *APIBase) unitsFromTag(tag string) ([]Unit, error) {
	unitTag, err := names.ParseUnitTag(tag)
	if err == nil {
		unit, err := api.backend.Unit(unitTag.Id())
		if err != nil {
			return nil, err
		}
		return []Unit{unit}, nil
	}
	appTag, err := names.ParseApplicationTag(tag)
	if err == nil {
		app, err := api.backend.Application(appTag.Id())
		if err != nil {
			return nil, err
		}
		return app.AllUnits()
	}
	return nil, fmt.Errorf("tag %q is neither unit nor application tag", tag)
}

// Builds a *params.UnitResult describing the unit argument.
func (api *APIBase) unitResultForUnit(unit Unit) (*params.UnitResult, error) {
	app, err := api.backend.Application(unit.ApplicationName())
	if err != nil {
		return nil, err
	}
	curl, _ := app.CharmURL()
	if curl == nil {
		return nil, errors.NotValidf("application charm url")
	}
	machineId, _ := unit.AssignedMachineId()
	workloadVersion, err := unit.WorkloadVersion()
	if err != nil {
		return nil, err
	}

	result := &params.UnitResult{
		Tag:             unit.Tag().String(),
		WorkloadVersion: workloadVersion,
		Machine:         machineId,
		Charm:           *curl,
		Life:            unit.Life().String(),
	}
	if machineId != "" {
		machine, err := api.backend.Machine(machineId)
		if err != nil {
			return nil, err
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
			return nil, err
		}
		result.OpenedPorts = openPorts
	}
	container, err := unit.ContainerInfo()
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
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
		return nil, err
	}
	return result, nil
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
			RelationId:       rel.Id(),
			Endpoint:         ep.Name,
			ApplicationData:  make(map[string]interface{}),
			UnitRelationData: make(map[string]params.RelationData),
		}
		relatedEps, err := rel.RelatedEndpoints(app.Name())
		if err != nil {
			return nil, errors.Trace(err)
		}
		// There is only one related endpoint.
		related := relatedEps[0]
		erd.RelatedEndpoint = related.Name

		appSettings, err := rel.ApplicationSettings(related.ApplicationName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for k, v := range appSettings {
			erd.ApplicationData[k] = v
		}

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

func checkCAASMinVersion(ch CharmMeta, caasVersion *version.Number) (err error) {
	// check caas min version.
	charmDeployment := ch.Meta().Deployment
	if caasVersion == nil || charmDeployment == nil || charmDeployment.MinVersion == "" {
		return nil
	}
	if len(strings.Split(charmDeployment.MinVersion, ".")) == 2 {
		// append build number if it's not specified.
		charmDeployment.MinVersion += ".0"
	}
	minver, err := version.Parse(charmDeployment.MinVersion)
	if err != nil {
		return errors.Trace(err)
	}
	if minver != version.Zero && minver.Compare(*caasVersion) > 0 {
		return errors.NewNotValid(nil, fmt.Sprintf(
			"charm requires a minimum k8s version of %v but the cluster only runs version %v",
			minver, caasVersion,
		))
	}
	return nil
}

// Leader returns the unit name of the leader for the given application.
func (api *APIBase) Leader(entity params.Entity) (params.StringResult, error) {
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

// DeployFromRepository for facade v18. The method was still not fully complete until v19.
// The NotImplemented error was for development purposes while use was behind a feature
// flag in the juju client.
func (api *APIv18) DeployFromRepository(args params.DeployFromRepositoryArgs) (params.DeployFromRepositoryResults, error) {
	return params.DeployFromRepositoryResults{}, errors.NotImplementedf("this facade method is under development")
}

// DeployFromRepository is a one-stop deployment method for repository
// charms. Only a charm name is required to deploy. If argument validation
// fails, a list of all errors found in validation will be returned. If a
// local resource is provided, details required for uploading the validated
// resource will be returned.
func (api *APIBase) DeployFromRepository(args params.DeployFromRepositoryArgs) (params.DeployFromRepositoryResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.DeployFromRepositoryResults{}, errors.Trace(err)
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return params.DeployFromRepositoryResults{}, errors.Trace(err)
	}

	results := make([]params.DeployFromRepositoryResult, len(args.Args))
	for i, entity := range args.Args {
		info, pending, errs := api.repoDeploy.DeployFromRepository(entity)
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
