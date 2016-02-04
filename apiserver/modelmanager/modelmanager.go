// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package modelmanager defines an API end point for functions
// dealing with models.  Creating, listing and sharing models.
package modelmanager

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.apiserver.modelmanager")

func init() {
	common.RegisterStandardFacade("ModelManager", 2, NewModelManagerAPI)
}

// ModelManager defines the methods on the modelmanager API end
// point.
type ModelManager interface {
	ConfigSkeleton(args params.ModelSkeletonConfigArgs) (params.ModelConfigResult, error)
	CreateModel(args params.ModelCreateArgs) (params.Model, error)
	ListModels(user params.Entity) (params.UserModelList, error)
}

// ModelManagerAPI implements the model manager interface and is
// the concrete implementation of the api end point.
type ModelManagerAPI struct {
	state       stateInterface
	authorizer  common.Authorizer
	toolsFinder *common.ToolsFinder
}

var _ ModelManager = (*ModelManagerAPI)(nil)

// NewModelManagerAPI creates a new api server endpoint for managing
// models.
func NewModelManagerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*ModelManagerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	urlGetter := common.NewToolsURLGetter(st.ModelUUID(), st)
	return &ModelManagerAPI{
		state:       getState(st),
		authorizer:  authorizer,
		toolsFinder: common.NewToolsFinder(st, st, urlGetter),
	}, nil
}

// authCheck checks if the user is acting on their own behalf, or if they
// are an administrator acting on behalf of another user.
func (em *ModelManagerAPI) authCheck(user names.UserTag) error {
	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := em.authorizer.GetAuthTag().(names.UserTag)
	isAdmin, err := em.state.IsControllerAdministrator(apiUser)
	if err != nil {
		return errors.Trace(err)
	}
	if isAdmin {
		logger.Tracef("%q is a controller admin", apiUser.Canonical())
		return nil
	}

	// We can't just compare the UserTags themselves as the provider part
	// may be unset, and gets replaced with 'local'. We must compare against
	// the Username of the user tag.
	if apiUser.Canonical() == user.Canonical() {
		return nil
	}
	return common.ErrPerm
}

// ConfigSource describes a type that is able to provide config.
// Abstracted primarily for testing.
type ConfigSource interface {
	Config() (*config.Config, error)
}

var configValuesFromController = []string{
	"type",
	"ca-cert",
	"state-port",
	"api-port",
}

// ConfigSkeleton returns config values to be used as a starting point for the
// API caller to construct a valid model specific config.  The provider
// and region params are there for future use, and current behaviour expects
// both of these to be empty.
func (em *ModelManagerAPI) ConfigSkeleton(args params.ModelSkeletonConfigArgs) (params.ModelConfigResult, error) {
	var result params.ModelConfigResult
	if args.Provider != "" {
		return result, errors.NotValidf("provider value %q", args.Provider)
	}
	if args.Region != "" {
		return result, errors.NotValidf("region value %q", args.Region)
	}

	controllerEnv, err := em.state.ControllerModel()
	if err != nil {
		return result, errors.Trace(err)
	}

	config, err := em.configSkeleton(controllerEnv)
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Config = config
	return result, nil
}

func (em *ModelManagerAPI) restrictedProviderFields(providerType string) ([]string, error) {
	provider, err := environs.Provider(providerType)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var fields []string
	fields = append(fields, configValuesFromController...)
	fields = append(fields, provider.RestrictedConfigAttributes()...)
	return fields, nil
}

func (em *ModelManagerAPI) configSkeleton(source ConfigSource) (map[string]interface{}, error) {
	baseConfig, err := source.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	baseMap := baseConfig.AllAttrs()

	fields, err := em.restrictedProviderFields(baseConfig.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result = make(map[string]interface{})
	for _, field := range fields {
		if value, found := baseMap[field]; found {
			result[field] = value
		}
	}
	return result, nil
}

func (em *ModelManagerAPI) checkVersion(cfg map[string]interface{}) error {
	// If there is no agent-version specified, use the current version.
	// otherwise we need to check for tools
	value, found := cfg["agent-version"]
	if !found {
		cfg["agent-version"] = version.Current.String()
		return nil
	}
	valuestr, ok := value.(string)
	if !ok {
		return errors.Errorf("agent-version must be a string but has type '%T'", value)
	}
	num, err := version.Parse(valuestr)
	if err != nil {
		return errors.Trace(err)
	}
	if comp := num.Compare(version.Current); comp > 0 {
		return errors.Errorf("agent-version cannot be greater than the server: %s", version.Current)
	} else if comp < 0 {
		// Look to see if we have tools available for that version.
		// Obviously if the version is the same, we have the tools available.
		list, err := em.toolsFinder.FindTools(params.FindToolsParams{
			Number: num,
		})
		if err != nil {
			return errors.Trace(err)
		}
		logger.Tracef("found tools: %#v", list)
		if len(list.List) == 0 {
			return errors.Errorf("no tools found for version %s", num)
		}
	}
	return nil
}

func (em *ModelManagerAPI) validConfig(attrs map[string]interface{}) (*config.Config, error) {
	cfg, err := config.New(config.UseDefaults, attrs)
	if err != nil {
		return nil, errors.Annotate(err, "creating config from values failed")
	}
	provider, err := environs.Provider(cfg.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err = provider.PrepareForCreateEnvironment(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err = provider.Validate(cfg, nil)
	if err != nil {
		return nil, errors.Annotate(err, "provider validation failed")
	}
	return cfg, nil
}

func (em *ModelManagerAPI) newModelConfig(args params.ModelCreateArgs, source ConfigSource) (*config.Config, error) {
	// For now, we just smash to the two maps together as we store
	// the account values and the model config together in the
	// *config.Config instance.
	joint := make(map[string]interface{})
	for key, value := range args.Config {
		joint[key] = value
	}
	// Account info overrides any config values.
	for key, value := range args.Account {
		joint[key] = value
	}
	if _, found := joint["uuid"]; found {
		return nil, errors.New("uuid is generated, you cannot specify one")
	}
	baseConfig, err := source.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	baseMap := baseConfig.AllAttrs()
	fields, err := em.restrictedProviderFields(baseConfig.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Before comparing any values, we need to push the config through
	// the provider validation code.  One of the reasons for this is that
	// numbers being serialized through JSON get turned into float64. The
	// schema code used in config will convert these back into integers.
	// However, before we can create a valid config, we need to make sure
	// we copy across fields from the main config that aren't there.
	for _, field := range fields {
		if _, found := joint[field]; !found {
			if baseValue, found := baseMap[field]; found {
				joint[field] = baseValue
			}
		}
	}

	// Generate the UUID for the server.
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate environment uuid")
	}
	joint["uuid"] = uuid.String()

	if err := em.checkVersion(joint); err != nil {
		return nil, errors.Annotate(err, "failed to create config")
	}

	// validConfig must only be called once.
	cfg, err := em.validConfig(joint)
	if err != nil {
		return nil, errors.Trace(err)
	}
	attrs := cfg.AllAttrs()
	// Any values that would normally be copied from the controller
	// config can also be defined, but if they differ from the controller
	// values, an error is returned.
	for _, field := range fields {
		if value, found := attrs[field]; found {
			if serverValue := baseMap[field]; value != serverValue {
				return nil, errors.Errorf(
					"specified %s \"%v\" does not match apiserver \"%v\"",
					field, value, serverValue)
			}
		}
	}

	return cfg, nil
}

// CreateModel creates a new model using the account and
// model config specified in the args.
func (em *ModelManagerAPI) CreateModel(args params.ModelCreateArgs) (params.Model, error) {
	result := params.Model{}
	// Get the controller model first. We need it both for the state
	// server owner and the ability to get the config.
	controllerEnv, err := em.state.ControllerModel()
	if err != nil {
		return result, errors.Trace(err)
	}

	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	// Any user is able to create themselves an model (until real fine
	// grain permissions are available), and admins (the creator of the state
	// server model) are able to create models for other people.
	err = em.authCheck(ownerTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	newConfig, err := em.newModelConfig(args, controllerEnv)
	if err != nil {
		return result, errors.Trace(err)
	}
	// NOTE: check the agent-version of the config, and if it is > the current
	// version, it is not supported, also check existing tools, and if we don't
	// have tools for that version, also die.
	model, st, err := em.state.NewModel(newConfig, ownerTag)
	if err != nil {
		return result, errors.Annotate(err, "failed to create new model")
	}
	defer st.Close()

	result.Name = model.Name()
	result.UUID = model.UUID()
	result.OwnerTag = model.Owner().String()

	return result, nil
}

// ListModels returns the models that the specified user
// has access to in the current server.  Only that controller owner
// can list models for any user (at this stage).  Other users
// can only ask about their own models.
func (em *ModelManagerAPI) ListModels(user params.Entity) (params.UserModelList, error) {
	result := params.UserModelList{}

	userTag, err := names.ParseUserTag(user.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}

	err = em.authCheck(userTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	models, err := em.state.ModelsForUser(userTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	for _, model := range models {
		var lastConn *time.Time
		userLastConn, err := model.LastConnection()
		if err != nil {
			if !state.IsNeverConnectedError(err) {
				return result, errors.Trace(err)
			}
		} else {
			lastConn = &userLastConn
		}
		result.UserModels = append(result.UserModels, params.UserModel{
			Model: params.Model{
				Name:     model.Name(),
				UUID:     model.UUID(),
				OwnerTag: model.Owner().String(),
			},
			LastConnection: lastConn,
		})
		logger.Debugf("list models: %s, %s, %s", model.Name(), model.UUID(), model.Owner())
	}

	return result, nil
}
