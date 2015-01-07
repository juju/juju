// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The environmentmanager package defines an API end point for functions
// dealing with envionments.  Creating, listing and sharing environments.
package environmentmanager

import (
	"github.com/juju/errors"
	"github.com/juju/juju/version"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.environmentmanager")

func init() {
	common.RegisterStandardFacade("EnvironmentManager", 0, NewEnvironmentManagerAPI)
}

// EnvironmentManager defines the methods on the environmentmanager API end
// point.
type EnvironmentManager interface {
	CreateEnvironment(args params.EnvironmentCreateArgs) (params.Environment, error)
	ListEnvironments(forUser string) (params.EnvironmentList, error)
}

// EnvironmentManagerAPI implements the environment manager interface and is
// the concrete implementation of the api end point.
type EnvironmentManagerAPI struct {
	state       *state.State
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
		state:       st,
		authorizer:  authorizer,
		toolsFinder: common.NewToolsFinder(st, st, urlGetter),
	}, nil
}

func (em *EnvironmentManagerAPI) authCheck(user, adminUser names.UserTag) (names.UserTag, error) {
	authTag := em.authorizer.GetAuthTag()
	apiUser, ok := authTag.(names.UserTag)
	if !ok {
		return apiUser, errors.Errorf("auth tag should be a user, but isn't: %q", authTag.String())
	}
	logger.Tracef("comparing api user %q against owner %q and admin %q", apiUser, user, adminUser)
	if apiUser == user || apiUser == adminUser {
		return apiUser, nil
	}
	return apiUser, common.ErrPerm
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
}

func (em *EnvironmentManagerAPI) checkVersion(joint map[string]interface{}) error {
	// If there is no agent-version specified, use the current version.
	// otherwise we need to check for tools
	if value, found := joint["agent-version"]; found {
		valuestr, ok := value.(string)
		if !ok {
			return errors.Errorf("agent-version must be a string but has type '%T'", value)
		}
		num, err := version.Parse(valuestr)
		if err != nil {
			return errors.Trace(err)
		}
		if comp := num.Compare(version.Current.Number); comp > 0 {
			return errors.Errorf("agent-version cannot be larger than the server: %s", version.Current.Number)
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
	} else {
		joint["agent-version"] = version.Current.Number.String()
	}
	return nil
}

func (em *EnvironmentManagerAPI) newEnvironmentConfig(args params.EnvironmentCreateArgs, source ConfigSource) (*config.Config, error) {
	// For now, we just smash to the two maps together as we store
	// the account values and the environment config together in the
	// *config.Config instance.
	joint := make(map[string]interface{})
	for key, value := range args.Account {
		joint[key] = value
	}
	for key, value := range args.Config {
		joint[key] = value
	}

	baseConfig, err := source.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	baseMap := baseConfig.AllAttrs()
	// Any values that would normally be copied from the state server
	// config can also be defined, but if they differ from the state server
	// values, an error is returned.
	for _, field := range configValuesFromStateServer {
		if value, found := joint[field]; found {
			if serverValue := baseMap[field]; value != serverValue {
				return nil, errors.Errorf(
					"specified %s \"%v\" does not match apiserver \"%v\"",
					field, value, serverValue)
			}
		} else {
			if value, found := baseMap[field]; found {
				joint[field] = value
			}
		}
	}
	if err := em.checkVersion(joint); err != nil {
		return nil, errors.Trace(err)
	}

	// Generate the UUID for the server.
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate environment uuid")
	}
	joint["uuid"] = uuid.String()
	cfg, err := config.New(config.UseDefaults, joint)
	if err != nil {
		return nil, errors.Trace(err)
	}
	provider, err := environs.Provider(cfg.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return provider.Validate(cfg, nil)
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
	adminUser := stateServerEnv.Owner()

	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	_, err = em.authCheck(ownerTag, adminUser)
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
func (em *EnvironmentManagerAPI) ListEnvironments(forUser string) (params.EnvironmentList, error) {
	result := params.EnvironmentList{}

	stateServerEnv, err := em.state.StateServerEnvironment()
	if err != nil {
		return result, errors.Trace(err)
	}
	adminUser := stateServerEnv.Owner()

	userTag, err := names.ParseUserTag(forUser)
	if err != nil {
		return result, errors.Trace(err)
	}

	_, err = em.authCheck(userTag, adminUser)
	if err != nil {
		return result, errors.Trace(err)
	}

	environments, err := em.state.EnvironmentsForUser(userTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	for _, env := range environments {
		result.Environments = append(result.Environments, params.Environment{
			Name:     env.Name(),
			UUID:     env.UUID(),
			OwnerTag: env.Owner().String(),
		})
	}

	return result, nil
}
