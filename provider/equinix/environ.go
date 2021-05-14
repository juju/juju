// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
	"github.com/juju/loggo"
	"github.com/juju/schema"
	"github.com/juju/version/v2"
	"gopkg.in/juju/environschema.v1"

	"github.com/packethost/packngo"
)

var logger = loggo.GetLogger("juju.provider.equinix")

type environConfig struct {
	config *config.Config
	attrs  map[string]interface{}
}

type environ struct {
	ecfgMutex     sync.Mutex
	ecfg          *environConfig
	name          string
	cloud         environscloudspec.CloudSpec
	equinixClient *packngo.Client
	namespace     instance.Namespace
}

var providerInstance environProvider

func (e *environ) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	return nil
}

func (e *environ) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, e, callCtx, args)
}

func (e *environ) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return nil, errors.NewNotImplemented(nil, "not implemented")
}

func (e *environ) AllRunningInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return nil, errors.NewNotImplemented(nil, "not implemented")
}

func (e *environ) Config() *config.Config {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	return e.ecfg.config
}

func (e *environ) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	return nil, errors.NewNotImplemented(nil, "not implemented")
}

func (e *environ) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	return nil, errors.NewNotImplemented(nil, "not implemented")
}

func (e *environ) Create(ctx context.ProviderCallContext, args environs.CreateParams) error {
	return nil
}

func (e *environ) Destroy(ctx context.ProviderCallContext) error {
	return errors.NewNotImplemented(nil, "not implemented")
}

func (e *environ) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	return errors.NewNotImplemented(nil, "not implemented")
}

func (e *environ) InstanceTypes(context.ProviderCallContext, constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	panic(errors.NewNotImplemented(nil, "not implemented"))
}

func (e *environ) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	panic(errors.NewNotImplemented(nil, "not implemented"))
}

func (e *environ) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	return nil
}

func (e *environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	e.name = controllerName
	return nil
}

func (*environ) Provider() environs.EnvironProvider {
	return &environProvider{}
}

func (e *environ) SetConfig(cfg *config.Config) error {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return errors.Annotate(err, "invalid config change")
	}
	e.ecfg = ecfg
	return nil
}

var configImmutableFields = []string{}
var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()
var configSchema = environschema.Fields{}
var configDefaults = schema.Defaults{}

func newConfig(cfg, old *config.Config) (*environConfig, error) {
	// Ensure that the provided config is valid.
	if err := config.Validate(cfg, old); err != nil {
		return nil, errors.Trace(err)
	}
	attrs, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if old != nil {
		// There's an old configuration. Validate it so that any
		// default values are correctly coerced for when we check
		// the old values later.
		oldEcfg, err := newConfig(old, nil)
		if err != nil {
			return nil, errors.Annotatef(err, "invalid base config")
		}
		for _, attr := range configImmutableFields {
			oldv, newv := oldEcfg.attrs[attr], attrs[attr]
			if oldv != newv {
				return nil, errors.Errorf(
					"%s: cannot change from %v to %v",
					attr, oldv, newv,
				)
			}
		}
	}

	ecfg := &environConfig{
		config: cfg,
		attrs:  attrs,
	}
	return ecfg, nil
}

func (e *environ) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (result *environs.StartInstanceResult, resultErr error) {
	panic(errors.NewNotImplemented(nil, "not implemented"))
}

func (e *environ) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	panic(errors.NewNotImplemented(nil, "not implemented"))
}

func (e *environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	return nil, errors.NotFoundf("storage provider %q", t)
}

func (e *environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	return nil, nil
}

// Region is specified in the HasRegion interface.
func (e *environ) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   e.cloud.Region,
		Endpoint: e.cloud.Endpoint,
	}, nil
}
