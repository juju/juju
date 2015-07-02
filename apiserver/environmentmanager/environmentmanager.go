// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The environmentmanager package defines an API end point for functions
// dealing with envionments.  Creating, listing and sharing environments.
package environmentmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
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
	AllEnvironments() (params.UserEnvironmentList, error)
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
func (em *EnvironmentManagerAPI) authCheck(user, adminUser names.UserTag) error {
	authTag := em.authorizer.GetAuthTag()
	apiUser, ok := authTag.(names.UserTag)
	if !ok {
		return errors.Errorf("auth tag should be a user, but isn't: %q", authTag.String())
	}
	// We can't just compare the UserTags themselves as the provider part
	// may be unset, and gets replaced with 'local'. We must compare against
	// the Username of the user tag.
	apiUsername := apiUser.Username()
	username := user.Username()
	adminUsername := adminUser.Username()
	logger.Tracef("comparing api user %q against owner %q and admin %q", apiUsername, username, adminUsername)
	if apiUsername == username || apiUsername == adminUsername {
		return nil
	}
	return common.ErrPerm
}

// environmentAuthCheck determines if the user has access to the current environment
// either by explicitly having access to the environment, or if they are a
// system administrator.
func (em *EnvironmentManagerAPI) environmentAuthCheck(st stateInterface) error {
	err := em.isSystemAdministrator()
	if err == nil {
		return err
	}

	// Not an admin, see if they have access the the specified environment
	authTag := em.authorizer.GetAuthTag()
	apiUserTag, ok := authTag.(names.UserTag)
	if !ok {
		return errors.Errorf("auth tag should be a user, but isn't: %q", authTag.String())
	}

	_, err = st.EnvironmentUser(apiUserTag)
	if err != nil {
		return common.ErrPerm
	}

	return nil
}

// isSystemAdministrator determines if the api user is a system administrator
func (em *EnvironmentManagerAPI) isSystemAdministrator() error {
	authTag := em.authorizer.GetAuthTag()
	apiUser, ok := authTag.(names.UserTag)
	if !ok {
		return errors.Errorf("auth tag should be a user, but isn't: %q", authTag.String())
	}

	isAdmin, err := em.state.IsSystemAdministrator(apiUser)
	if err != nil {
		return errors.Trace(err)
	}
	if !isAdmin {
		return common.ErrPerm
	}

	return nil
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
	adminUser := stateServerEnv.Owner()

	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	// Any user is able to create themselves an environment (until real fine
	// grain permissions are available), and admins (the creator of the state
	// server environment) are able to create environments for other people.
	err = em.authCheck(ownerTag, adminUser)
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

	stateServerEnv, err := em.state.StateServerEnvironment()
	if err != nil {
		return result, errors.Trace(err)
	}
	adminUser := stateServerEnv.Owner()

	userTag, err := names.ParseUserTag(user.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}

	err = em.authCheck(userTag, adminUser)
	if err != nil {
		return result, errors.Trace(err)
	}

	environments, err := em.state.EnvironmentsForUser(userTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	for _, env := range environments {
		result.UserEnvironments = append(result.UserEnvironments, params.UserEnvironment{
			Environment: params.Environment{
				Name:     env.Name(),
				UUID:     env.UUID(),
				OwnerTag: env.Owner().String(),
			},
			LastConnection: env.LastConnection,
		})
		logger.Debugf("list env: %s, %s, %s", env.Name(), env.UUID(), env.Owner())
	}

	return result, nil
}

// AllEnvironments allows system administrators to get the list of all the
// environments in the system.
func (em *EnvironmentManagerAPI) AllEnvironments() (params.UserEnvironmentList, error) {
	result := params.UserEnvironmentList{}

	authTag := em.authorizer.GetAuthTag()
	apiUser, ok := authTag.(names.UserTag)
	if !ok {
		return result, errors.Errorf("auth tag should be a user, but isn't: %q", authTag.String())
	}

	err := em.isSystemAdministrator()
	if err != nil {
		return result, errors.Trace(err)
	}

	// Get all the environments that the authenticated user can see, and
	// supplement that with the other environments that exist that the user
	// cannot see. The reason we do this is to get the LastConnection time for
	// the environments that the user is able to see, so we have consistent
	// output when listing with or without --all when an admin user.
	visibleEnvironments, err := em.ListEnvironments(params.Entity{Tag: apiUser.String()})
	if err != nil {
		return result, errors.Trace(err)
	}

	envs := make(map[string]params.UserEnvironment)
	for _, env := range visibleEnvironments.UserEnvironments {
		envs[env.UUID] = env
	}

	allEnvs, err := em.state.AllEnvironments()
	if err != nil {
		return result, errors.Trace(err)
	}

	for _, env := range allEnvs {
		if _, ok := envs[env.UUID()]; !ok {
			envs[env.UUID()] = params.UserEnvironment{
				Environment: params.Environment{
					Name:     env.Name(),
					UUID:     env.UUID(),
					OwnerTag: env.Owner().String(),
				},
				// No LastConnection as this user hasn't.
			}
		}
	}

	for _, userEnv := range envs {
		result.UserEnvironments = append(result.UserEnvironments, userEnv)
	}

	return result, nil
}

// ListBlockedEnvironments returns a list of all environments on the system which
// have a block in place.  Callers must be system administrators to retrieve the list.
func (em *EnvironmentManagerAPI) ListBlockedEnvironments() (params.EnvironmentBlockInfoList, error) {
	results := params.EnvironmentBlockInfoList{}

	// Check that we are authorized
	err := em.isSystemAdministrator()
	if err != nil {
		return results, errors.Trace(err)
	}

	blocks, err := em.state.AllBlocksForSystem()
	if err != nil {
		return results, errors.Trace(err)
	}

	envBlocks := make(map[string][]string)
	for _, block := range blocks {
		uuid := block.EnvUUID()
		types, ok := envBlocks[uuid]
		if !ok {
			types = []string{block.Type().String()}
		} else {
			types = append(types, block.Type().String())
		}
		envBlocks[uuid] = types
	}

	for uuid, blocks := range envBlocks {
		envInfo, err := em.state.GetEnvironment(names.NewEnvironTag(uuid))
		if err != nil {
			logger.Debugf("Unable to get name for environment: %s", uuid)
			continue
		}
		results.Environments = append(results.Environments, params.EnvironmentBlockInfo{
			Environment: params.Environment{
				UUID:       envInfo.UUID(),
				Name:       envInfo.Name(),
				OwnerTag:   envInfo.Owner().String(),
				ServerUUID: envInfo.ServerUUID(),
			},
			Blocks: blocks,
		})
	}

	return results, nil
}

func (em *EnvironmentManagerAPI) DestroySystem(args params.DestroySystemArgs) error {
	// Check we're destroying the system env
	st := em.state
	if args.EnvUUID != st.EnvironUUID() {
		return errors.Errorf("%q is not a system", args.EnvUUID)
	}

	// Check that we are authorized
	err := em.isSystemAdministrator()
	if err != nil {
		return errors.Trace(err)
	}

	// Get list of all environments in the system.
	allEnvs, err := st.AllEnvironments()
	if err != nil {
		return errors.Trace(err)
	}

	// If there are hosted environments and KillEnvs was not specified, don't
	// bother trying to destroy the system, as it will fail.
	if len(allEnvs) > 1 && !args.KillEnvs {
		return errors.Errorf("state server environment cannot be destroyed before all other environments are destroyed")
	}

	// Check for blocks
	blocks, err := st.AllBlocksForSystem()
	if err != nil {
		logger.Debugf("Unable to get blocks for system: %s", err)
		if !args.IgnoreBlocks {
			return errors.Trace(err)
		}
	}
	if len(blocks) > 0 {
		if !args.IgnoreBlocks {
			return common.ErrOperationBlocked("found blocks in system environments")
		}

		err := st.RemoveAllBlocksForSystem()
		if err != nil {
			return errors.Trace(err)
		}
	}

	if args.KillEnvs {
		for _, env := range allEnvs {
			if env.UUID() != st.EnvironUUID() {
				err = em.DestroyEnvironment(params.DestroyEnvironmentArgs{EnvUUID: env.UUID()})
				if err != nil {
					logger.Warningf("unable to destroy environment %q: %s", env.UUID(), err)
				}
			}
		}
	}

	return em.DestroyEnvironment(params.DestroyEnvironmentArgs{EnvUUID: st.EnvironUUID()})
}

// DestroyEnvironment destroys all services and non-manager machine
// instances in the specified environment.
func (em *EnvironmentManagerAPI) DestroyEnvironment(args params.DestroyEnvironmentArgs) (err error) {
	st := em.state
	logger.Warningf("Destroy environment called: %s\n", args.EnvUUID)
	envUUID := args.EnvUUID
	if envUUID != em.state.EnvironUUID() {
		envTag := names.NewEnvironTag(envUUID)
		if st, err = em.state.ForEnviron(envTag); err != nil {
			return errors.Trace(err)
		}
		defer st.Close()
	}

	err = em.environmentAuthCheck(st)
	if err != nil {
		return errors.Trace(err)
	}

	check := common.NewBlockChecker(st)
	if err = check.DestroyAllowed(); err != nil {
		return errors.Trace(err)
	}

	env, err := st.Environment()
	if err != nil {
		return errors.Trace(err)
	}

	if err = env.Destroy(); err != nil {
		return errors.Trace(err)
	}

	machines, err := st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}

	// We must destroy instances server-side to support JES (Juju Environment
	// Server), as there's no CLI to fall back on. In that case, we only ever
	// destroy non-state machines; we leave destroying state servers in non-
	// hosted environments to the CLI, as otherwise the API server may get cut
	// off.
	if err := destroyInstances(st, machines); err != nil {
		return errors.Trace(err)
	}

	// If this is not the state server environment, remove all documents from
	// state associated with the environment.
	if env.UUID() != env.ServerTag().Id() {
		return errors.Trace(st.RemoveAllEnvironDocs())
	}

	// Return to the caller. If it's the CLI, it will finish up
	// by calling the provider's Destroy method, which will
	// destroy the state servers, any straggler instances, and
	// other provider-specific resources.
	return nil
}

// destroyInstances directly destroys all non-manager,
// non-manual machine instances.
func destroyInstances(st stateInterface, machines []*state.Machine) error {
	var ids []instance.Id
	for _, m := range machines {
		if m.IsManager() {
			continue
		}
		if _, isContainer := m.ParentId(); isContainer {
			continue
		}
		manual, err := m.IsManual()
		if manual {
			continue
		} else if err != nil {
			return err
		}
		id, err := m.InstanceId()
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	envcfg, err := st.EnvironConfig()
	if err != nil {
		return err
	}
	env, err := environs.New(envcfg)
	if err != nil {
		return err
	}
	return env.StopInstances(ids...)
}

// EnvironmentGet returns the environment config for the system
// environment.  For information on the current environment, use
// client.EnvironmentGet
func (em *EnvironmentManagerAPI) EnvironmentGet() (_ params.EnvironmentConfigResults, err error) {
	result := params.EnvironmentConfigResults{}

	st := em.state
	stateServerEnv, err := em.state.StateServerEnvironment()
	if err != nil {
		return result, errors.Trace(err)
	}

	// We need to obtain the state for the stateServerEnvironment to
	// determine if the caller is authorized to access the environment.
	if stateServerEnv.UUID() != st.EnvironUUID() {
		envTag := names.NewEnvironTag(stateServerEnv.UUID())
		st, err = em.state.ForEnviron(envTag)
		if err != nil {
			return result, errors.Trace(err)
		}
		defer st.Close()
	}

	err = em.environmentAuthCheck(st)
	if err != nil {
		return result, errors.Trace(err)
	}

	config, err := stateServerEnv.Config()
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Config = config.AllAttrs()
	return result, nil
}
