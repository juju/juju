// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"github.com/juju/version"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envcontext "github.com/juju/juju/environs/context"
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

func (e *Environ) allInstances(tags map[string]string) ([]*ociInstance, error) {
	compartment := e.ecfg().compartmentID()
	request := ociCore.ListInstancesRequest{
		CompartmentId: compartment,
	}
	response, err := e.Compute.ListInstances(context.Background(), request)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ret := []*ociInstance{}
	for _, val := range response.Items {
		if val.LifecycleState == ociCore.InstanceLifecycleStateTerminated {
			continue
		}
		missingTag := false
		for i, j := range tags {
			tagVal, ok := val.FreeformTags[i]
			if !ok || tagVal != j {
				missingTag = true
				break
			}
		}
		if missingTag {
			// One of the tags was not found
			continue
		}
		inst, err := newInstance(val, e)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ret = append(ret, inst)
	}
	return ret, nil
}

func (e *Environ) getOCIInstance(id instance.Id) (*ociInstance, error) {
	instanceId := string(id)
	request := ociCore.GetInstanceRequest{
		InstanceId: &instanceId,
	}

	response, err := e.Compute.GetInstance(context.Background(), request)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return newInstance(response.Instance, e)
}

func (e *Environ) isNotFound(response *http.Response) bool {
	if response.StatusCode == http.StatusNotFound {
		return true
	}
	return false
}

// waitForResourceStatus will ping the resource until the fetch function returns true,
// the timeout is reached, or an error occurs.
func (o *Environ) waitForResourceStatus(
	statusFunc func(resID *string) (status string, err error),
	resId *string, desiredStatus string,
	timeout time.Duration,
) error {

	var status string
	var err error
	timeoutTimer := o.clock.NewTimer(timeout)
	defer timeoutTimer.Stop()

	retryTimer := o.clock.NewTimer(0)
	defer retryTimer.Stop()

	for {
		select {
		case <-retryTimer.Chan():
			status, err = statusFunc(resId)
			if err != nil {
				return err
			}
			if status == desiredStatus {
				return nil
			}
			retryTimer.Reset(2 * time.Second)
		case <-timeoutTimer.Chan():
			return errors.Errorf(
				"timed out waiting for resource %q to transition to %v. Current status: %q",
				*resId, desiredStatus, status,
			)
		}
	}
}

// AvailabilityZones is defined in the common.ZonedEnviron interface
func (e *Environ) AvailabilityZones(ctx envcontext.ProviderCallContext) ([]common.AvailabilityZone, error) {
	return nil, errors.NotImplementedf("AvailabilityZones")
}

// InstanceAvailabilityzoneNames implements common.ZonedEnviron.
func (e *Environ) InstanceAvailabilityZoneNames(ctx envcontext.ProviderCallContext, ids []instance.Id) ([]string, error) {
	return nil, errors.NotImplementedf("InstanceAvailabilityZoneNames")
}

// DeriveAvailabilityZones implements common.ZonedEnviron.
func (e *Environ) DeriveAvailabilityZones(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
	return nil, errors.NotImplementedf("DeriveAvailabilityZones")
}

// Instances implements environs.Environ.
func (e *Environ) Instances(ctx envcontext.ProviderCallContext, ids []instance.Id) ([]instance.Instance, error) {
	return nil, errors.NotImplementedf("Instances")
}

// PrepareForBootstrap implements environs.Environ.
func (e *Environ) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	return errors.NotImplementedf("PrepareForBootstrap")
}

// Bootstrap implements environs.Environ.
func (e *Environ) Bootstrap(ctx environs.BootstrapContext, callCtx envcontext.ProviderCallContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, e, callCtx, params)
}

// Create implements environs.Environ.
func (e *Environ) Create(ctx envcontext.ProviderCallContext, params environs.CreateParams) error {
	return errors.NotImplementedf("Create")
}

// AdoptResources implements environs.Environ.
func (e *Environ) AdoptResources(ctx envcontext.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
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
func (e *Environ) ControllerInstances(ctx envcontext.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	return nil, errors.NotImplementedf("ControllerInstances")
}

// Destroy implements environs.Environ.
func (e *Environ) Destroy(ctx envcontext.ProviderCallContext) error {
	return common.Destroy(e, ctx)
}

// DestroyController implements environs.Environ.
func (e *Environ) DestroyController(ctx envcontext.ProviderCallContext, controllerUUID string) error {
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
func (e *Environ) StartInstance(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	return nil, errors.NotImplementedf("StartInstance")
}

// StopInstances implements environs.InstanceBroker.
func (e *Environ) StopInstances(ctx envcontext.ProviderCallContext, ids ...instance.Id) error {
	return errors.NotImplementedf("StopInstances")
}

// AllInstances implements environs.InstanceBroker.
func (e *Environ) AllInstances(ctx envcontext.ProviderCallContext) ([]instance.Instance, error) {
	return nil, errors.NotImplementedf("AllInstances")
}

// MaintainInstance implements environs.InstanceBroker.
func (e *Environ) MaintainInstance(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) error {
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
func (e *Environ) PrecheckInstance(envcontext.ProviderCallContext, environs.PrecheckInstanceParams) error {
	return errors.NotImplementedf("PrecheckInstance")
}

// InstanceTypes implements environs.InstancePrechecker.
func (e *Environ) InstanceTypes(envcontext.ProviderCallContext, constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	return instances.InstanceTypesWithCostMetadata{}, errors.NotImplementedf("InstanceTypes")
}
