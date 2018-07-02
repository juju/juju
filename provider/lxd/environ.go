// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"sync"

	"github.com/juju/errors"
	lxdclient "github.com/lxc/lxd/client"
	lxdapi "github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

const bootstrapMessage = `To configure your system to better support LXD containers, please see: https://github.com/lxc/lxd/blob/master/doc/production-setup.md`

// Server defines an interface of all localized methods that the environment
// and provider utilizes.
//go:generate mockgen -package lxd -destination environ_mock_test.go github.com/juju/juju/provider/lxd Server,ServerFactory
type Server interface {
	FindImage(string, string, []lxd.ServerSpec, bool, environs.StatusCallbackFunc) (lxd.SourcedImage, error)
	GetServer() (server *lxdapi.Server, ETag string, err error)
	GetConnectionInfo() (info *lxdclient.ConnectionInfo, err error)
	UpdateServerConfig(map[string]string) error
	UpdateContainerConfig(string, map[string]string) error
	GetCertificate(fingerprint string) (certificate *lxdapi.Certificate, ETag string, err error)
	DeleteCertificate(fingerprint string) (err error)
	CreateClientCertificate(certificate *lxd.Certificate) error
	LocalBridgeName() string
	AliveContainers(prefix string) ([]lxd.Container, error)
	ContainerAddresses(name string) ([]network.Address, error)
	RemoveContainer(name string) error
	RemoveContainers(names []string) error
	FilterContainers(prefix string, statuses ...string) ([]lxd.Container, error)
	CreateContainerFromSpec(spec lxd.ContainerSpec) (*lxd.Container, error)
	WriteContainer(*lxd.Container) error
	CreateProfileWithConfig(string, map[string]string) error
	HasProfile(string) (bool, error)
	StorageSupported() bool
	GetStoragePool(name string) (pool *lxdapi.StoragePool, ETag string, err error)
	GetStoragePools() (pools []lxdapi.StoragePool, err error)
	CreatePool(name, driver string, attrs map[string]string) error
	GetStoragePoolVolume(pool string, volType string, name string) (*lxdapi.StorageVolume, string, error)
	GetStoragePoolVolumes(pool string) (volumes []lxdapi.StorageVolume, err error)
	CreateVolume(pool, name string, config map[string]string) error
	UpdateStoragePoolVolume(pool string, volType string, name string, volume lxdapi.StorageVolumePut, ETag string) error
	DeleteStoragePoolVolume(pool string, volType string, name string) (err error)
	ServerCertificate() string
}

// ServerFactory creates a new factory for creating servers that are required
// by the server.
type ServerFactory interface {
	// LocalServer creates a new lxd server and augments and wraps the lxd
	// server, by ensuring sane defaults exist with network, storage.
	LocalServer() (Server, error)
	// RemoteServer creates a new server that connects to a remote lxd server.
	// If the cloudSpec endpoint is nil or empty, it will assume that you want
	// to connection to a local server and will instead use that one.
	RemoteServer(environs.CloudSpec) (Server, error)
}

type baseProvider interface {
	// BootstrapEnv bootstraps a Juju environment.
	BootstrapEnv(environs.BootstrapContext, context.ProviderCallContext, environs.BootstrapParams) (*environs.BootstrapResult, error)

	// DestroyEnv destroys the provided Juju environment.
	DestroyEnv(ctx context.ProviderCallContext) error
}

type environ struct {
	cloud    environs.CloudSpec
	provider *environProvider

	name   string
	uuid   string
	server Server
	base   baseProvider

	// namespace is used to create the machine and device hostnames.
	namespace instance.Namespace

	lock sync.Mutex
	ecfg *environConfig
}

func newEnviron(
	_ *environProvider,
	local bool,
	spec environs.CloudSpec,
	cfg *config.Config,
	serverFactory ServerFactory,
) (*environ, error) {
	ecfg, err := newValidConfig(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}

	namespace, err := instance.NewNamespace(cfg.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	server, err := serverFactory.RemoteServer(spec)
	if err != nil {
		return nil, errors.Trace(err)
	}

	env := &environ{
		cloud:     spec,
		name:      ecfg.Name(),
		uuid:      ecfg.UUID(),
		server:    server,
		namespace: namespace,
		ecfg:      ecfg,
	}
	env.base = common.DefaultProvider{Env: env}

	//TODO(wwitzel3) make sure we are also cleaning up profiles during destroy
	if err := env.initProfile(); err != nil {
		return nil, errors.Trace(err)
	}

	return env, nil
}

var defaultProfileConfig = map[string]string{
	"boot.autostart":   "true",
	"security.nesting": "true",
}

func (env *environ) initProfile() error {
	hasProfile, err := env.server.HasProfile(env.profileName())
	if err != nil {
		return errors.Trace(err)
	}

	if hasProfile {
		return nil
	}

	return env.server.CreateProfileWithConfig(env.profileName(), defaultProfileConfig)
}

func (env *environ) profileName() string {
	return "juju-" + env.ecfg.Name()
}

// Name returns the name of the environment.
func (env *environ) Name() string {
	return env.name
}

// Provider returns the environment provider that created this env.
func (env *environ) Provider() environs.EnvironProvider {
	return env.provider
}

// SetConfig updates the env's configuration.
func (env *environ) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()
	ecfg, err := newValidConfig(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	env.ecfg = ecfg
	return nil
}

// Config returns the configuration data with which the env was created.
func (env *environ) Config() *config.Config {
	env.lock.Lock()
	cfg := env.ecfg.Config
	env.lock.Unlock()
	return cfg
}

// PrepareForBootstrap implements environs.Environ.
func (env *environ) PrepareForBootstrap(ctx environs.BootstrapContext) error {
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
		return errors.Trace(err)
	}
	if env.storageSupported() {
		if err := destroyModelFilesystems(env); err != nil {
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
		return errors.Trace(err)
	}
	if env.storageSupported() {
		if err := destroyControllerFilesystems(env, controllerUUID); err != nil {
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

	logger.Debugf("removing instances: %v", instances)
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

	return errors.Trace(env.server.RemoveContainers(names))
}
