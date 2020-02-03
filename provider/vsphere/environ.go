// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"fmt"
	"path"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/version"
	"github.com/vmware/govmomi/vim25/mo"
	"golang.org/x/net/context"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	callcontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
)

// Note: This provider/environment does *not* implement storage.

type environ struct {
	name     string
	cloud    environs.CloudSpec
	provider *environProvider

	// namespace is used to create the machine and device hostnames.
	namespace instance.Namespace

	lock sync.Mutex // lock protects access the following fields.
	ecfg *environConfig
}

func newEnviron(
	provider *environProvider,
	cloud environs.CloudSpec,
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
		name:      ecfg.Name(),
		cloud:     cloud,
		provider:  provider,
		ecfg:      ecfg,
		namespace: namespace,
	}
	return env, nil
}

func (env *environ) withClient(ctx context.Context, callCtx callcontext.ProviderCallContext, f func(Client) error) error {
	client, err := env.dialClient(ctx)
	if err != nil {
		// LP #1849194: this is a case at bootstrap time, where a connection
		// to vsphere failed. It can be wrong Credentials only, differently
		// from all the other HandleCredentialError cases
		common.HandleCredentialError(IsAuthorisationFailure, err, callCtx)
		return errors.Annotate(err, "dialing client")
	}
	defer client.Close(ctx)
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
func (env *environ) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()

	if env.ecfg == nil {
		return errors.New("cannot set config on uninitialized env")
	}

	if err := env.ecfg.update(cfg); err != nil {
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
func (env *environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	return nil
}

// Create implements environs.Environ.
func (env *environ) Create(ctx callcontext.ProviderCallContext, args environs.CreateParams) error {
	return env.withSession(ctx, func(env *sessionEnviron) error {
		return env.Create(ctx, args)
	})
}

// Create implements environs.Environ.
func (env *sessionEnviron) Create(ctx callcontext.ProviderCallContext, args environs.CreateParams) error {
	return env.ensureVMFolder(args.ControllerUUID, ctx)
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
	if err := env.withSession(callCtx, func(env *sessionEnviron) error {
		return env.ensureVMFolder(args.ControllerConfig.ControllerUUID(), callCtx)
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return Bootstrap(ctx, env, callCtx, args)
}

func (env *sessionEnviron) Bootstrap(
	ctx environs.BootstrapContext,
	callCtx callcontext.ProviderCallContext,
	args environs.BootstrapParams,
) (result *environs.BootstrapResult, err error) {
	return nil, errors.Errorf("sessionEnviron.Bootstrap should never be called")
}

func (env *sessionEnviron) ensureVMFolder(controllerUUID string, ctx callcontext.ProviderCallContext) error {
	_, err := env.client.EnsureVMFolder(env.ctx, env.getVMFolder(), path.Join(
		controllerFolderName(controllerUUID),
		env.modelFolderName(),
	))
	HandleCredentialError(err, env, ctx)
	return errors.Trace(err)
}

// DestroyEnv is exported, because it has to be rewritten in external unit tests.
var DestroyEnv = common.Destroy

// AdoptResources is part of the Environ interface.
func (env *environ) AdoptResources(ctx callcontext.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	// Move model folder into the controller's folder.
	return env.withSession(ctx, func(env *sessionEnviron) error {
		return env.AdoptResources(ctx, controllerUUID, fromVersion)
	})
}

// AdoptResources is part of the Environ interface.
func (env *sessionEnviron) AdoptResources(ctx callcontext.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	err := env.client.MoveVMFolderInto(env.ctx,
		path.Join(env.getVMFolder(), controllerFolderName(controllerUUID)),
		path.Join(env.getVMFolder(), controllerFolderName("*"), env.modelFolderName()),
	)
	HandleCredentialError(err, env, ctx)
	return err
}

// Destroy is part of the environs.Environ interface.
func (env *environ) Destroy(ctx callcontext.ProviderCallContext) error {
	return env.withSession(ctx, func(env *sessionEnviron) error {
		return env.Destroy(ctx)
	})
}

// Destroy is part of the environs.Environ interface.
func (env *sessionEnviron) Destroy(ctx callcontext.ProviderCallContext) error {
	if err := DestroyEnv(env, ctx); err != nil {
		// We don't need to worry about handling credential errors
		// here - this is implemented in terms of common operations
		// that call back into this provider, so we'll handle them
		// further down the stack.
		return errors.Trace(err)
	}
	err := env.client.DestroyVMFolder(env.ctx,
		path.Join(env.getVMFolder(), controllerFolderName("*"), env.modelFolderName()),
	)
	HandleCredentialError(err, env, ctx)
	return err
}

// DestroyController implements the Environ interface.
func (env *environ) DestroyController(ctx callcontext.ProviderCallContext, controllerUUID string) error {
	return env.withSession(ctx, func(env *sessionEnviron) error {
		return env.DestroyController(ctx, controllerUUID)
	})
}

// DestroyController implements the Environ interface.
func (env *sessionEnviron) DestroyController(ctx callcontext.ProviderCallContext, controllerUUID string) error {
	if err := env.Destroy(ctx); err != nil {
		return errors.Trace(err)
	}
	controllerFolderName := controllerFolderName(controllerUUID)
	if err := env.client.RemoveVirtualMachines(env.ctx,
		path.Join(env.getVMFolder(), controllerFolderName, modelFolderName("*", "*"), "*"),
	); err != nil {
		HandleCredentialError(err, env, ctx)
		return errors.Annotate(err, "removing VMs")
	}
	if err := env.client.DestroyVMFolder(env.ctx, path.Join(env.getVMFolder(), controllerFolderName)); err != nil {
		HandleCredentialError(err, env, ctx)
		return errors.Annotate(err, "destroying VM folder")
	}

	// Remove VMDK cache(s). The user can specify the datastore, and can
	// change it over time; or if not specified, any accessible datastore
	// will be used. We must check them all.
	datastores, err := env.accessibleDatastores(ctx)
	if err != nil {
		return errors.Annotate(err, "listing datastores")
	}
	for _, ds := range datastores {
		datastorePath := fmt.Sprintf("[%s] %s", ds.Name, templateDirectoryName(env.getVMFolder(), controllerUUID))
		logger.Debugf("deleting: %s", datastorePath)
		if err := env.client.DeleteDatastoreFile(env.ctx, datastorePath); err != nil {
			HandleCredentialError(err, env, ctx)
			return errors.Annotatef(err, "deleting VMDK cache from datastore %q", ds.Name)
		}
	}
	return nil
}

func (env *sessionEnviron) getVMFolder() string {
	return env.environ.cloud.Credential.Attributes()[credAttrVMFolder]
}

func (env *sessionEnviron) accessibleDatastores(ctx callcontext.ProviderCallContext) ([]*mo.Datastore, error) {
	datastores, err := env.client.Datastores(env.ctx)
	if err != nil {
		HandleCredentialError(err, env, ctx)
		return nil, errors.Trace(err)
	}
	var results []*mo.Datastore
	for _, ds := range datastores {
		if !ds.Summary.Accessible {
			continue
		}
		results = append(results, ds)
	}
	return results, nil
}
