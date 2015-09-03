// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The environmentmanager package defines an API end point for functions
// dealing with envionments.  Creating, listing and sharing environments.
package environmentmanager

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
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.apiserver.environmentmanager")

func init() {
	common.RegisterStandardFacadeForFeature("EnvironmentManager", 1, NewEnvironmentManagerAPI, feature.JES)
}

// EnvironmentManager defines the methods on the environmentmanager API end
// point.
type EnvironmentManager interface {
	ConfigSkeleton(args params.EnvironmentSkeletonConfigArgs) (params.EnvironConfigResult, error)
	CreateEnvironment(args params.EnvironmentCreateArgs) (params.Environment, error)
	ListEnvironments(user params.Entity) (params.UserEnvironmentList, error)
}

// EnvironmentManagerAPI implements the environment manager interface and is
// the concrete implementation of the api end point.
type EnvironmentManagerAPI struct {
	state       stateInterface
	authorizer  common.Authorizer
	toolsFinder *common.ToolsFinder
}

var _ EnvironmentManager = (*EnvironmentManagerAPI)(nil)

// NewEnvironmentManagerAPI creates a new api server endpoint for managing
// environments.
func NewEnvironmentManagerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*EnvironmentManagerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	urlGetter := common.NewToolsURLGetter(st.EnvironUUID(), st)
	return &EnvironmentManagerAPI{
		state:       getState(st),
		authorizer:  authorizer,
		toolsFinder: common.NewToolsFinder(st, st, urlGetter),
	}, nil
}

// authCheck checks if the user is acting on their own behalf, or if they
// are an administrator acting on behalf of another user.
func (em *EnvironmentManagerAPI) authCheck(user names.UserTag) error {
	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := em.authorizer.GetAuthTag().(names.UserTag)
	isAdmin, err := em.state.IsSystemAdministrator(apiUser)
	if err != nil {
		return errors.Trace(err)
	}
	if isAdmin {
		logger.Tracef("%q is a system admin", apiUser.Username())
		return nil
	}

	// We can't just compare the UserTags themselves as the provider part
	// may be unset, and gets replaced with 'local'. We must compare against
	// the Username of the user tag.
	if apiUser.Username() == user.Username() {
		return nil
	}
	return common.ErrPerm
}

// ConfigSource describes a type that is able to provide config.
// Abstracted primarily for testing.
type ConfigSource interface {
	Config() (*config.Config, error)
}

var configValuesFromStateServer = []string{
	"type",
	"ca-cert",
	"state-port",
	"api-port",
	"syslog-port",
	"rsyslog-ca-cert",
	"rsyslog-ca-key",
}

// ConfigSkeleton returns config values to be used as a starting point for the
// API caller to construct a valid environment specific config.  The provider
// and region params are there for future use, and current behaviour expects
// both of these to be empty.
func (em *EnvironmentManagerAPI) ConfigSkeleton(args params.EnvironmentSkeletonConfigArgs) (params.EnvironConfigResult, error) {
	var result params.EnvironConfigResult
	if args.Provider != "" {
		return result, errors.NotValidf("provider value %q", args.Provider)
	}
	if args.Region != "" {
		return result, errors.NotValidf("region value %q", args.Region)
	}

	stateServerEnv, err := em.state.StateServerEnvironment()
	if err != nil {
		return result, errors.Trace(err)
	}

	config, err := em.configSkeleton(stateServerEnv)
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Config = config
	return result, nil
}

func (em *EnvironmentManagerAPI) restrictedProviderFields(providerType string) ([]string, error) {
	provider, err := environs.Provider(providerType)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var fields []string
	fields = append(fields, configValuesFromStateServer...)
	fields = append(fields, provider.RestrictedConfigAttributes()...)
	return fields, nil
}

func (em *EnvironmentManagerAPI) configSkeleton(source ConfigSource) (map[string]interface{}, error) {
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

func (em *EnvironmentManagerAPI) checkVersion(cfg map[string]interface{}) error {
	// If there is no agent-version specified, use the current version.
	// otherwise we need to check for tools
	value, found := cfg["agent-version"]
	if !found {
		cfg["agent-version"] = version.Current.Number.String()
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
	if comp := num.Compare(version.Current.Number); comp > 0 {
		return errors.Errorf("agent-version cannot be greater than the server: %s", version.Current.Number)
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

func (em *EnvironmentManagerAPI) validConfig(attrs map[string]interface{}) (*config.Config, error) {
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

func (em *EnvironmentManagerAPI) newEnvironmentConfig(args params.EnvironmentCreateArgs, source ConfigSource) (*config.Config, error) {
	// For now, we just smash to the two maps together as we store
	// the account values and the environment config together in the
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

	cfg, err := em.validConfig(joint)
	if err != nil {
		return nil, errors.Trace(err)
	}
	attrs := cfg.AllAttrs()
	// Any values that would normally be copied from the state server
	// config can also be defined, but if they differ from the state server
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
	if err := em.checkVersion(attrs); err != nil {
		return nil, errors.Trace(err)
	}

	// Generate the UUID for the server.
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate environment uuid")
	}
	attrs["uuid"] = uuid.String()

	return em.validConfig(attrs)
}

// CreateEnvironment creates a new environment using the account and
// environment config specified in the args.
func (em *EnvironmentManagerAPI) CreateEnvironment(args params.EnvironmentCreateArgs) (params.Environment, error) {
	result := params.Environment{}
	// Get the state server environment first. We need it both for the state
	// server owner and the ability to get the config.
	stateServerEnv, err := em.state.StateServerEnvironment()
	if err != nil {
		return result, errors.Trace(err)
	}

	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	// Any user is able to create themselves an environment (until real fine
	// grain permissions are available), and admins (the creator of the state
	// server environment) are able to create environments for other people.
	err = em.authCheck(ownerTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	newConfig, err := em.newEnvironmentConfig(args, stateServerEnv)
	if err != nil {
		return result, errors.Trace(err)
	}
	// NOTE: check the agent-version of the config, and if it is > the current
	// version, it is not supported, also check existing tools, and if we don't
	// have tools for that version, also die.
	env, st, err := em.state.NewEnvironment(newConfig, ownerTag)
	if err != nil {
		return result, errors.Annotate(err, "failed to create new environment")
	}
	defer st.Close()

	result.Name = env.Name()
	result.UUID = env.UUID()
	result.OwnerTag = env.Owner().String()

	return result, nil
}

// ListEnvironments returns the environments that the specified user
// has access to in the current server.  Only that state server owner
// can list environments for any user (at this stage).  Other users
// can only ask about their own environments.
func (em *EnvironmentManagerAPI) ListEnvironments(user params.Entity) (params.UserEnvironmentList, error) {
	result := params.UserEnvironmentList{}

	userTag, err := names.ParseUserTag(user.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}

	err = em.authCheck(userTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	environments, err := em.state.EnvironmentsForUser(userTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	for _, env := range environments {
		var lastConn *time.Time
		userLastConn, err := env.LastConnection()
		if err != nil {
			if !state.IsNeverConnectedError(err) {
				return result, errors.Trace(err)
			}
		} else {
			lastConn = &userLastConn
		}
		result.UserEnvironments = append(result.UserEnvironments, params.UserEnvironment{
			Environment: params.Environment{
				Name:     env.Name(),
				UUID:     env.UUID(),
				OwnerTag: env.Owner().String(),
			},
			LastConnection: lastConn,
		})
		logger.Debugf("list env: %s, %s, %s", env.Name(), env.UUID(), env.Owner())
	}

	return result, nil
}
