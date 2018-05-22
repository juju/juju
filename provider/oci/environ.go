// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"net/http"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"github.com/juju/version"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	providerCommon "github.com/juju/juju/provider/oci/common"
	"github.com/juju/juju/storage"

	ociCore "github.com/oracle/oci-go-sdk/core"
)

type Environ struct {
	Compute    providerCommon.OCIComputeClient
	Networking providerCommon.OCINetworkingClient
	Storage    providerCommon.OCIStorageClient
	Firewall   providerCommon.OCIFirewallClient
	Identity   providerCommon.OCIIdentityClient
	p          *EnvironProvider
	clock      clock.Clock
	ecfgMutex  sync.Mutex
	ecfgObj    *environConfig
	namespace  instance.Namespace

	vcn     ociCore.Vcn
	seclist ociCore.SecurityList
	// subnets contains one subnet for each availability domain
	// these will get created once the environment is spun up, and
	// will never change.
	subnets map[string][]ociCore.Subnet
}

var _ common.ZonedEnviron = (*Environ)(nil)
var _ storage.ProviderRegistry = (*Environ)(nil)
var _ environs.Environ = (*Environ)(nil)
var _ environs.Firewaller = (*Environ)(nil)
var _ environs.Networking = (*Environ)(nil)
var _ environs.NetworkingEnviron = (*Environ)(nil)

func (e *Environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	return e.ecfgObj
}

func (e *Environ) isNotFound(response *http.Response) bool {
	if response.StatusCode == http.StatusNotFound {
		return true
	}
	return false
}

// AvailabilityZones is defined in the common.ZonedEnviron interface
func (e *Environ) AvailabilityZones(ctx context.ProviderCallContext) ([]common.AvailabilityZone, error) {
	return nil, errors.NotImplementedf("AvailabilityZones")
}

// InstanceAvailabilityzoneNames implements common.ZonedEnviron.
func (e *Environ) InstanceAvailabilityZoneNames(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
	return nil, errors.NotImplementedf("InstanceAvailabilityZoneNames")
}

// DeriveAvailabilityZones implements common.ZonedEnviron.
func (e *Environ) DeriveAvailabilityZones(ctx context.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
	return nil, errors.NotImplementedf("DeriveAvailabilityZones")
}

// Instances implements environs.Environ.
func (e *Environ) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instance.Instance, error) {
	return nil, errors.NotImplementedf("Instances")
}

// PrepareForBootstrap implements environs.Environ.
func (e *Environ) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	return errors.NotImplementedf("PrepareForBootstrap")
}

// Bootstrap implements environs.Environ.
func (e *Environ) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, e, callCtx, params)
}

// Create implements environs.Environ.
func (e *Environ) Create(ctx context.ProviderCallContext, params environs.CreateParams) error {
	return errors.NotImplementedf("Create")
}

// AdoptResources implements environs.Environ.
func (e *Environ) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	return errors.NotImplementedf("AdoptResources")
}

// ConstraintsValidator implements environs.Environ.
func (e *Environ) ConstraintsValidator() (constraints.Validator, error) {
	return nil, errors.NotImplementedf("ConstraintsValidator")
}

// SetConfig implements environs.Environ.
func (e *Environ) SetConfig(cfg *config.Config) error {
	ecfg, err := e.p.newConfig(cfg)
	if err != nil {
		return err
	}

	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	e.ecfgObj = ecfg

	return nil
}

// ControllerInstances implements environs.Environ.
func (e *Environ) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	return nil, errors.NotImplementedf("ControllerInstances")
}

// Destroy implements environs.Environ.
func (e *Environ) Destroy(ctx context.ProviderCallContext) error {
	return common.Destroy(e, ctx)
}

// DestroyController implements environs.Environ.
func (e *Environ) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	return errors.NotImplementedf("DestroyController")
}

// Provider implements environs.Environ.
func (e *Environ) Provider() environs.EnvironProvider {
	return nil
}

// StorageProviderTypes implements storage.ProviderRegistry.
func (e *Environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{}, errors.NotImplementedf("StorageProviderTypes")
}

// StorageProvider implements storage.ProviderRegistry.
func (e *Environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	return nil, errors.NotFoundf("storage provider %q", t)
}

// StartInstance implements environs.InstanceBroker.
func (e *Environ) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	return nil, errors.NotImplementedf("StartInstance")
}

// StopInstances implements environs.InstanceBroker.
func (e *Environ) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	return errors.NotImplementedf("StopInstances")
}

// AllInstances implements environs.InstanceBroker.
func (e *Environ) AllInstances(ctx context.ProviderCallContext) ([]instance.Instance, error) {
	return nil, errors.NotImplementedf("AllInstances")
}

// MaintainInstance implements environs.InstanceBroker.
func (e *Environ) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	return errors.NotImplementedf("MaintainInstance")
}

// Config implements environs.ConfigGetter.
func (e *Environ) Config() *config.Config {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	if e.ecfgObj == nil {
		return nil
	}
	return e.ecfgObj.Config
}

// PrecheckInstance implements environs.InstancePrechecker.
func (e *Environ) PrecheckInstance(context.ProviderCallContext, environs.PrecheckInstanceParams) error {
	return errors.NotImplementedf("PrecheckInstance")
}

// InstanceTypes implements environs.InstancePrechecker.
func (e *Environ) InstanceTypes(context.ProviderCallContext, constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	return instances.InstanceTypesWithCostMetadata{}, errors.NotImplementedf("InstanceTypes")
}
