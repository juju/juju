// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	stdcontext "context"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/provider/common"
)

const bootstrapMessage = `To configure your system to better support LXD containers, please see: https://linuxcontainers.org/lxd/docs/master/explanation/performance_tuning/`

type baseProvider interface {
	// BootstrapEnv bootstraps a Juju environment.
	BootstrapEnv(environs.BootstrapContext, context.ProviderCallContext, environs.BootstrapParams) (*environs.BootstrapResult, error)

	// DestroyEnv destroys the provided Juju environment.
	DestroyEnv(ctx context.ProviderCallContext) error
}

type environ struct {
	environs.NoSpaceDiscoveryEnviron

	cloud    environscloudspec.CloudSpec
	provider *environProvider

	name string
	uuid string
	base baseProvider

	// namespace is used to create the machine and device hostnames.
	namespace instance.Namespace

	// lock protects the *Unlocked fields below.
	lock           sync.Mutex
	ecfgUnlocked   *environConfig
	serverUnlocked Server

	// profileMutex is used when writing profiles via the server.
	profileMutex sync.Mutex
}

func newEnviron(
	p *environProvider,
	spec environscloudspec.CloudSpec,
	cfg *config.Config,
) (*environ, error) {
	ecfg, err := newValidConfig(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}

	namespace, err := instance.NewNamespace(cfg.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	env := &environ{
		provider:     p,
		cloud:        spec,
		name:         ecfg.Name(),
		uuid:         ecfg.UUID(),
		namespace:    namespace,
		ecfgUnlocked: ecfg,
	}
	env.base = common.DefaultProvider{Env: env}

	err = env.SetCloudSpec(stdcontext.TODO(), spec)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return env, nil
}

func (env *environ) initProfile() error {
	pName := env.profileName()

	hasProfile, err := env.serverUnlocked.HasProfile(pName)
	if err != nil {
		return errors.Trace(err)
	}
	if hasProfile {
		return nil
	}

	cfg := map[string]string{
		"boot.autostart":   "true",
		"security.nesting": "true",
	}

	// In ci, perhaps other places, there can be a race if more than one
	// controller is starting up, where we try to create the profile more
	// than once and get: The profile already exists.  LXD does not have
	// typed errors. Therefore if CreateProfile fails, check to see if the
	// profile exists.  No need to fail if it does.
	err = env.serverUnlocked.CreateProfileWithConfig(pName, cfg)
	if err == nil {
		return nil
	}
	hasProfile, hasErr := env.serverUnlocked.HasProfile(pName)
	if hasErr != nil {
		logger.Errorf("%s", err)
		return errors.Trace(hasErr)
	}
	if hasProfile {
		logger.Debugf("received %q, but no need to fail", err)
		return nil
	}
	return err
}

func (env *environ) profileName() string {
	return "juju-" + env.Name()
}

// Name returns the name of the environ.
func (env *environ) Name() string {
	return env.name
}

// Provider returns the provider that created this environ.
func (env *environ) Provider() environs.EnvironProvider {
	return env.provider
}

// SetConfig updates the environ's configuration.
func (env *environ) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()
	ecfg, err := newValidConfig(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	env.ecfgUnlocked = ecfg
	return nil
}

// SetCloudSpec is specified in the environs.Environ interface.
func (env *environ) SetCloudSpec(_ stdcontext.Context, spec environscloudspec.CloudSpec) error {
	env.lock.Lock()
	defer env.lock.Unlock()

	serverFactory := env.provider.serverFactory
	server, err := serverFactory.RemoteServer(spec)
	if err != nil {
		return errors.Trace(err)
	}

	if project := env.ecfgUnlocked.project(); project != "" {
		server.UseProject(project)
	}

	env.serverUnlocked = server
	return env.initProfile()
}

func (env *environ) server() Server {
	env.lock.Lock()
	defer env.lock.Unlock()

	return env.serverUnlocked
}

// Config returns the configuration data with which the env was created.
func (env *environ) Config() *config.Config {
	env.lock.Lock()
	defer env.lock.Unlock()

	cfg := env.ecfgUnlocked.Config
	return cfg
}

// ValidateCloudEndpoint returns nil if the current model can talk to the lxd
// server endpoint.  Used as validation during model upgrades.
// Implements environs.CloudEndpointChecker
func (env *environ) ValidateCloudEndpoint(ctx context.ProviderCallContext) error {
	info, err := env.server().GetConnectionInfo()
	if err != nil {
		return err
	}
	err = env.provider.Ping(ctx, info.URL)
	return errors.Trace(err)
}

// PrepareForBootstrap implements environs.Environ.
func (env *environ) PrepareForBootstrap(_ environs.BootstrapContext, _ string) error {
	return nil
}

// Create implements environs.Environ.
func (env *environ) Create(context.ProviderCallContext, environs.CreateParams) error {
	return nil
}

// Bootstrap implements environs.Environ.
func (env *environ) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	ctx.Infof("%s", bootstrapMessage)
	return env.base.BootstrapEnv(ctx, callCtx, params)
}

// Destroy shuts down all known machines and destroys the rest of the
// known environment.
func (env *environ) Destroy(ctx context.ProviderCallContext) error {
	if err := env.base.DestroyEnv(ctx); err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return errors.Trace(err)
	}
	if env.storageSupported() {
		if err := destroyModelFilesystems(env); err != nil {
			common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
			return errors.Annotate(err, "destroying LXD filesystems for model")
		}
	}
	return nil
}

// DestroyController implements the Environ interface.
func (env *environ) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	if err := env.Destroy(ctx); err != nil {
		return errors.Trace(err)
	}
	if err := env.destroyHostedModelResources(controllerUUID); err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return errors.Trace(err)
	}
	if env.storageSupported() {
		if err := destroyControllerFilesystems(env, controllerUUID); err != nil {
			common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
			return errors.Annotate(err, "destroying LXD filesystems for controller")
		}
	}
	return nil
}

func (env *environ) destroyHostedModelResources(controllerUUID string) error {
	// Destroy all instances with juju-controller-uuid
	// matching the specified UUID.
	const prefix = "juju-"
	instances, err := env.prefixedInstances(prefix)
	if err != nil {
		return errors.Annotate(err, "listing instances")
	}

	var names []string
	for _, inst := range instances {
		if inst.container.Metadata(tags.JujuModel) == env.uuid {
			continue
		}
		if inst.container.Metadata(tags.JujuController) != controllerUUID {
			continue
		}
		names = append(names, string(inst.Id()))
	}
	logger.Debugf("removing instances: %v", names)

	return errors.Trace(env.server().RemoveContainers(names))
}

// lxdAvailabilityZone wraps a LXD cluster member as an availability zone.
type lxdAvailabilityZone struct {
	api.ClusterMember
}

// Name implements AvailabilityZone.
func (z *lxdAvailabilityZone) Name() string {
	return z.ServerName
}

// Available implements AvailabilityZone.
func (z *lxdAvailabilityZone) Available() bool {
	return strings.ToLower(z.Status) == "online"
}

// AvailabilityZones (ZonedEnviron) returns all availability zones in the
// environment. For LXD, this means the cluster node names.
func (env *environ) AvailabilityZones(ctx context.ProviderCallContext) (network.AvailabilityZones, error) {
	// If we are not using a clustered server (which includes those not
	// supporting the clustering API) just represent the single server as the
	// only availability zone.
	server := env.server()
	if !server.IsClustered() {
		return network.AvailabilityZones{
			&lxdAvailabilityZone{
				ClusterMember: api.ClusterMember{
					ServerName: server.Name(),
					Status:     "ONLINE",
				},
			},
		}, nil
	}

	nodes, err := server.GetClusterMembers()
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, errors.Annotate(err, "listing cluster members")
	}
	aZones := make(network.AvailabilityZones, len(nodes))
	for i, n := range nodes {
		aZones[i] = &lxdAvailabilityZone{n}
	}
	return aZones, nil
}

// InstanceAvailabilityZoneNames (ZonedEnviron) returns the names of the
// availability zones for the specified instances.
// For containers, this means the LXD server node names where they reside.
func (env *environ) InstanceAvailabilityZoneNames(
	ctx context.ProviderCallContext, ids []instance.Id,
) (map[instance.Id]string, error) {
	instances, err := env.Instances(ctx, ids)
	if err != nil && err != environs.ErrPartialInstances {
		return nil, err
	}

	// If not clustered, just report all input IDs as being in the zone
	// represented by the single server.
	server := env.server()
	if !server.IsClustered() {
		zones := make(map[instance.Id]string, len(ids))
		n := server.Name()
		for _, id := range ids {
			zones[id] = n
		}
		return zones, nil
	}

	zones := make(map[instance.Id]string, len(instances))
	for _, ins := range instances {
		if ei, ok := ins.(*environInstance); ok {
			zones[ins.Id()] = ei.container.Location
		}
	}
	return zones, nil
}

// DeriveAvailabilityZones (ZonedEnviron) attempts to derive availability zones
// from the specified StartInstanceParams.
func (env *environ) DeriveAvailabilityZones(
	ctx context.ProviderCallContext, args environs.StartInstanceParams,
) ([]string, error) {
	p, err := env.parsePlacement(ctx, args.Placement)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if p.nodeName == "" {
		return nil, nil
	}
	return []string{p.nodeName}, nil
}

// TODO: HML 2-apr-2019
// When provisioner_task processProfileChanges() is
// removed, maybe change to take an lxdprofile.ProfilePost as
// an arg.
// MaybeWriteLXDProfile implements environs.LXDProfiler.
func (env *environ) MaybeWriteLXDProfile(pName string, put lxdprofile.Profile) error {
	env.profileMutex.Lock()
	defer env.profileMutex.Unlock()
	server := env.server()
	hasProfile, err := server.HasProfile(pName)
	if err != nil {
		return errors.Trace(err)
	}
	if hasProfile {
		logger.Debugf("lxd profile %q already exists, not written again", pName)
		return nil
	}
	logger.Debugf("attempting to write lxd profile %q %+v", pName, put)
	post := api.ProfilesPost{
		Name: pName,
		ProfilePut: api.ProfilePut{
			Description: put.Description,
			Config:      put.Config,
			Devices:     put.Devices,
		},
	}
	if err = server.CreateProfile(post); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("wrote lxd profile %q", pName)
	if err := env.verifyProfile(pName); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// verifyProfile gets the actual profile from lxd for the name provided
// and logs the result. For informational purposes only. Returns an error
// if the call to GetProfile fails.
func (env *environ) verifyProfile(pName string) error {
	// As there are configs where we do not have the option of looking at
	// the profile on the machine to verify, verify here that what we thought
	// was written, is what was written.
	profile, _, err := env.server().GetProfile(pName)
	if err != nil {
		return err
	}
	logger.Debugf("lxd profile %q: received %+v ", pName, profile.ProfilePut)
	return nil
}

// LXDProfileNames implements environs.LXDProfiler.
func (env *environ) LXDProfileNames(containerName string) ([]string, error) {
	return env.server().GetContainerProfiles(containerName)
}

// AssignLXDProfiles implements environs.LXDProfiler.
func (env *environ) AssignLXDProfiles(instID string, profilesNames []string, profilePosts []lxdprofile.ProfilePost) (current []string, err error) {
	report := func(err error) ([]string, error) {
		// Always return the current profiles assigned to the instance.
		currentProfiles, err2 := env.LXDProfileNames(instID)
		if err != nil && err2 != nil {
			logger.Errorf("retrieving profile names for %q: %s", instID, err2)
		}
		return currentProfiles, err
	}

	// Write any new profilePosts and gather a slice of profile
	// names to be deleted, after removal.
	var deleteProfiles []string
	for _, p := range profilePosts {
		if p.Profile != nil {
			if err := env.MaybeWriteLXDProfile(p.Name, *p.Profile); err != nil {
				return report(err)
			}
		} else {
			deleteProfiles = append(deleteProfiles, p.Name)
		}
	}

	server := env.server()
	if err := server.UpdateContainerProfiles(instID, profilesNames); err != nil {
		return report(errors.Trace(err))
	}

	for _, name := range deleteProfiles {
		if err := server.DeleteProfile(name); err != nil {
			// most likely the failure is because the profile is already in use
			logger.Debugf("failed to delete profile %q: %s", name, err)
		}
	}
	return report(nil)
}
