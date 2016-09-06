// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/tools/lxdclient"
)

type baseProvider interface {
	// BootstrapEnv bootstraps a Juju environment.
	BootstrapEnv(environs.BootstrapContext, environs.BootstrapParams) (*environs.BootstrapResult, error)

	// DestroyEnv destroys the provided Juju environment.
	DestroyEnv() error
}

type environ struct {
	name string
	uuid string
	raw  *rawProvider
	base baseProvider

	// namespace is used to create the machine and device hostnames.
	namespace instance.Namespace

	lock sync.Mutex
	ecfg *environConfig
}

type newRawProviderFunc func(environs.CloudSpec) (*rawProvider, error)

func newEnviron(spec environs.CloudSpec, cfg *config.Config, newRawProvider newRawProviderFunc) (*environ, error) {
	ecfg, err := newValidConfig(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}

	namespace, err := instance.NewNamespace(cfg.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	raw, err := newRawProvider(spec)
	if err != nil {
		return nil, errors.Trace(err)
	}

	env := &environ{
		name:      ecfg.Name(),
		uuid:      ecfg.UUID(),
		raw:       raw,
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
	hasProfile, err := env.raw.HasProfile(env.profileName())
	if err != nil {
		return errors.Trace(err)
	}

	if hasProfile {
		return nil
	}

	return env.raw.CreateProfile(env.profileName(), defaultProfileConfig)
}

func (env *environ) profileName() string {
	return "juju-" + env.ecfg.Name()
}

// Name returns the name of the environment.
func (env *environ) Name() string {
	return env.name
}

// Provider returns the environment provider that created this env.
func (*environ) Provider() environs.EnvironProvider {
	return providerInstance
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
	if err := lxdclient.EnableHTTPSListener(env.raw); err != nil {
		return errors.Annotate(err, "enabling HTTPS listener")
	}
	return nil
}

// Create implements environs.Environ.
func (env *environ) Create(environs.CreateParams) error {
	return nil
}

// Bootstrap implements environs.Environ.
func (env *environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	// Using the Bootstrap func from provider/common should be fine.
	// Local provider does its own thing because it has to deal directly
	// with localhost rather than using SSH.
	return env.base.BootstrapEnv(ctx, params)
}

// Destroy shuts down all known machines and destroys the rest of the
// known environment.
func (env *environ) Destroy() error {
	ports, err := env.Ports()
	if err != nil {
		return errors.Trace(err)
	}
	if len(ports) > 0 {
		if err := env.ClosePorts(ports); err != nil {
			return errors.Trace(err)
		}
	}
	if err := env.base.DestroyEnv(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// DestroyController implements the Environ interface.
func (env *environ) DestroyController(controllerUUID string) error {
	if err := env.Destroy(); err != nil {
		return errors.Trace(err)
	}
	return env.destroyHostedModelResources(controllerUUID)
}

func (env *environ) destroyHostedModelResources(controllerUUID string) error {
	// Destroy all instances where juju-controller-uuid,
	// but not juju-model-uuid, matches env.uuid.
	prefix := env.namespace.Prefix()
	instances, err := env.prefixedInstances(prefix)
	if err != nil {
		return errors.Annotate(err, "listing instances")
	}
	logger.Debugf("instances: %v", instances)
	var names []string
	for _, inst := range instances {
		metadata := inst.raw.Metadata()
		if metadata[tags.JujuModel] == env.uuid {
			continue
		}
		if metadata[tags.JujuController] != controllerUUID {
			continue
		}
		names = append(names, string(inst.Id()))
	}
	if err := env.raw.RemoveInstances(prefix, names...); err != nil {
		return errors.Annotate(err, "removing hosted model instances")
	}
	return nil
}
