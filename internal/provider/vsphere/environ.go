// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"path"
	"sync"

	"github.com/juju/errors"
	"github.com/vmware/govmomi/vim25/mo"
	"golang.org/x/net/context"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	callcontext "github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/provider/common"
)

// Note: This provider/environment does *not* implement storage.

type environ struct {
	common.CredentialInvalidator

	name     string
	cloud    environscloudspec.CloudSpec
	provider *environProvider

	// namespace is used to create the machine and device hostnames.
	namespace instance.Namespace

	lock sync.Mutex // lock protects access the following fields.
	ecfg *environConfig
}

func newEnviron(
	ctx context.Context,
	provider *environProvider,
	invalidator environs.CredentialInvalidator,
	cloud environscloudspec.CloudSpec,
	cfg *config.Config,
) (*environ, error) {
	ecfg, err := newValidConfig(ctx, cfg)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}

	namespace, err := instance.NewNamespace(cfg.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	env := &environ{
		CredentialInvalidator: common.NewCredentialInvalidator(invalidator, IsAuthorisationFailure),
		name:                  ecfg.Name(),
		cloud:                 cloud,
		provider:              provider,
		ecfg:                  ecfg,
		namespace:             namespace,
	}
	return env, nil
}

func (env *environ) withClient(ctx context.Context, f func(Client) error) error {
	client, err := env.dialClient(ctx)
	if err != nil {
		// LP #1849194: this is a case at bootstrap time, where a connection
		// to vsphere failed. It can be wrong Credentials only, differently
		// from all the other HandleCredentialError cases
		return errors.Annotate(env.HandleCredentialError(ctx, err), "dialing client")
	}
	defer func() { _ = client.Close(ctx) }()
	return f(client)
}

func (env *environ) dialClient(ctx context.Context) (Client, error) {
	return dialClient(ctx, env.cloud, env.provider.dial)
}

// Name is part of the environs.Environ interface.
func (env *environ) Name() string {
	return env.name
}

// Provider is part of the environs.Environ interface.
func (env *environ) Provider() environs.EnvironProvider {
	return env.provider
}

// SetConfig is part of the environs.Environ interface.
func (env *environ) SetConfig(ctx context.Context, cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()

	if env.ecfg == nil {
		return errors.New("cannot set config on uninitialized env")
	}

	if err := env.ecfg.update(ctx, cfg); err != nil {
		return errors.Annotate(err, "invalid config change")
	}
	return nil
}

// Config is part of the environs.Environ interface.
func (env *environ) Config() *config.Config {
	env.lock.Lock()
	cfg := env.ecfg.Config
	env.lock.Unlock()
	return cfg
}

// PrepareForBootstrap implements environs.Environ.
func (env *environ) PrepareForBootstrap(environs.BootstrapContext, string) error {
	return nil
}

// ValidateProviderForNewModel is part of the [environs.ModelResources] interface.
func (env *environ) ValidateProviderForNewModel(context.Context) error {
	return nil
}

// CreateModelResources is part of the [environs.ModelResources] interface.
func (env *environ) CreateModelResources(ctx context.Context, args environs.CreateParams) error {
	return env.withSession(ctx, func(senv *sessionEnviron) error {
		return senv.CreateModelResources(ctx, args)
	})
}

func (senv *sessionEnviron) ValidateProviderForNewModel(context.Context) error {
	return nil
}

// CreateModelResources is part of the [environs.ModelResources] interface.
func (senv *sessionEnviron) CreateModelResources(ctx context.Context, args environs.CreateParams) error {
	return senv.ensureVMFolder(args.ControllerUUID, ctx)
}

// Bootstrap is exported, because it has to be rewritten in external unit tests
var Bootstrap = common.Bootstrap

// Bootstrap is part of the environs.Environ interface.
func (env *environ) Bootstrap(
	ctx environs.BootstrapContext,
	callCtx callcontext.ProviderCallContext,
	args environs.BootstrapParams,
) (result *environs.BootstrapResult, err error) {
	// NOTE(axw) we must not pass a sessionEnviron to common.Bootstrap,
	// as the Environ will be used during instance finalization after
	// the Bootstrap method returns, and the session will be invalid.
	if err := env.withSession(callCtx, func(senv *sessionEnviron) error {
		return senv.ensureVMFolder(args.ControllerConfig.ControllerUUID(), callCtx)
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return Bootstrap(ctx, env, callCtx, args)
}

func (senv *sessionEnviron) Bootstrap(
	ctx environs.BootstrapContext,
	callCtx callcontext.ProviderCallContext,
	args environs.BootstrapParams,
) (result *environs.BootstrapResult, err error) {
	return nil, errors.Errorf("sessionEnviron.Bootstrap should never be called")
}

func (senv *sessionEnviron) ensureVMFolder(controllerUUID string, ctx context.Context) error {
	_, err := senv.client.EnsureVMFolder(senv.ctx, senv.getVMFolder(), path.Join(
		controllerFolderName(controllerUUID),
		senv.modelFolderName(),
	))
	return errors.Trace(senv.handleCredentialError(ctx, err))
}

// DestroyEnv is exported, because it has to be rewritten in external unit tests.
var DestroyEnv = common.Destroy

// AdoptResources is part of the Environ interface.
func (env *environ) AdoptResources(ctx callcontext.ProviderCallContext, controllerUUID string, fromVersion semversion.Number) error {
	// Move model folder into the controller's folder.
	return env.withSession(ctx, func(senv *sessionEnviron) error {
		return senv.AdoptResources(ctx, controllerUUID, fromVersion)
	})
}

// AdoptResources is part of the Environ interface.
func (senv *sessionEnviron) AdoptResources(ctx callcontext.ProviderCallContext, controllerUUID string, fromVersion semversion.Number) error {
	err := senv.client.MoveVMFolderInto(senv.ctx,
		path.Join(senv.getVMFolder(), controllerFolderName(controllerUUID)),
		path.Join(senv.getVMFolder(), controllerFolderName("*"), senv.modelFolderName()),
	)
	return senv.handleCredentialError(ctx, err)
}

// Destroy is part of the environs.Environ interface.
func (env *environ) Destroy(ctx callcontext.ProviderCallContext) error {
	return env.withSession(ctx, func(senv *sessionEnviron) error {
		return senv.Destroy(ctx)
	})
}

// Destroy is part of the environs.Environ interface.
func (senv *sessionEnviron) Destroy(ctx callcontext.ProviderCallContext) error {
	if err := DestroyEnv(senv, ctx); err != nil {
		// We don't need to worry about handling credential errors
		// here - this is implemented in terms of common operations
		// that call back into this provider, so we'll handle them
		// further down the stack.
		return errors.Trace(err)
	}
	err := senv.client.DestroyVMFolder(senv.ctx,
		path.Join(senv.getVMFolder(), controllerFolderName("*"), senv.modelFolderName()),
	)
	return senv.handleCredentialError(ctx, err)
}

// DestroyController implements the Environ interface.
func (env *environ) DestroyController(ctx callcontext.ProviderCallContext, controllerUUID string) error {
	return env.withSession(ctx, func(senv *sessionEnviron) error {
		return senv.DestroyController(ctx, controllerUUID)
	})
}

// DestroyController implements the Environ interface.
func (senv *sessionEnviron) DestroyController(ctx callcontext.ProviderCallContext, controllerUUID string) error {
	if err := senv.Destroy(ctx); err != nil {
		return errors.Trace(err)
	}
	controllerFolderName := controllerFolderName(controllerUUID)
	if err := senv.client.RemoveVirtualMachines(senv.ctx,
		path.Join(senv.getVMFolder(), controllerFolderName, modelFolderName("*", "*"), "*"),
	); err != nil {
		return errors.Annotate(senv.handleCredentialError(ctx, err), "removing VMs")
	}
	if err := senv.client.DestroyVMFolder(senv.ctx, path.Join(senv.getVMFolder(), controllerFolderName)); err != nil {
		return errors.Annotate(senv.handleCredentialError(ctx, err), "destroying VM folder")
	}
	return nil
}

func (senv *sessionEnviron) getVMFolder() string {
	return senv.environ.cloud.Credential.Attributes()[credAttrVMFolder]
}

func (senv *sessionEnviron) accessibleDatastores(ctx callcontext.ProviderCallContext) ([]mo.Datastore, error) {
	datastores, err := senv.client.Datastores(senv.ctx)
	if err != nil {
		return nil, senv.handleCredentialError(ctx, err)
	}
	var results []mo.Datastore
	for _, ds := range datastores {
		if !ds.Summary.Accessible {
			continue
		}
		results = append(results, ds)
	}
	return results, nil
}
