// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	envcontext "github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/common"
	ocicommon "github.com/juju/juju/internal/provider/oci/common"
)

const (
	// MinVolumeSizeMB is the minimum size in MB for a volume or boot disk
	MinVolumeSizeMB = 51200

	// MaxVolumeSizeMB is the maximum size in MB for a volume or boot disk
	MaxVolumeSizeMB = 16777216
)

type ociInstance struct {
	raw  ociCore.Instance
	env  *Environ
	etag *string

	newInstanceConfigurator func(string) common.InstanceConfigurator

	mutex sync.Mutex
}

type vnicWithIndex struct {
	Vnic ociCore.Vnic
	Idx  int
}

var _ instances.Instance = (*ociInstance)(nil)
var maxPollIterations = 30
var pollTime = 10 * time.Second

var statusMap = map[ociCore.InstanceLifecycleStateEnum]status.Status{
	ociCore.InstanceLifecycleStateProvisioning:  status.Provisioning,
	ociCore.InstanceLifecycleStateRunning:       status.Running,
	ociCore.InstanceLifecycleStateStarting:      status.Provisioning,
	ociCore.InstanceLifecycleStateStopping:      status.Running,
	ociCore.InstanceLifecycleStateStopped:       status.Running,
	ociCore.InstanceLifecycleStateCreatingImage: status.Provisioning,
	ociCore.InstanceLifecycleStateTerminating:   status.Running,
	ociCore.InstanceLifecycleStateTerminated:    status.Running,
}

// newInstance returns a new oracleInstance
func newInstance(raw ociCore.Instance, env *Environ) (*ociInstance, error) {
	if raw.Id == nil {
		return nil, errors.New(
			"Instance response does not contain an ID",
		)
	}

	return &ociInstance{
		raw:                     raw,
		env:                     env,
		newInstanceConfigurator: common.NewSshInstanceConfigurator,
	}, nil
}

// SetInstance sets the raw property of ociInstance{}
// Testing purposes.
func (o *ociInstance) SetInstance(inst ociCore.Instance) {
	o.raw = inst
}

func (o *ociInstance) availabilityZone() string {
	return *o.raw.AvailabilityDomain
}

// Id implements instances.Instance
func (o *ociInstance) Id() instance.Id {
	return instance.Id(*o.raw.Id)
}

// Status implements instances.Instance
func (o *ociInstance) Status(ctx envcontext.ProviderCallContext) instance.Status {
	if err := o.refresh(); err != nil {
		ocicommon.HandleCredentialError(err, ctx)
		return instance.Status{}
	}
	state, ok := statusMap[o.raw.LifecycleState]
	if !ok {
		state = status.Unknown
	}
	return instance.Status{
		Status:  state,
		Message: strings.ToLower(string(o.raw.LifecycleState)),
	}
}

func (o *ociInstance) getVnics() ([]vnicWithIndex, error) {
	attachments, err := o.env.Compute.ListVnicAttachments(context.Background(), o.raw.CompartmentId, o.raw.Id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	nics := []vnicWithIndex{}
	for _, val := range attachments {
		vnicID := val.VnicId
		request := ociCore.GetVnicRequest{
			VnicId: vnicID,
		}
		response, err := o.env.Networking.GetVnic(context.Background(), request)
		if err != nil {
			return nil, errors.Trace(err)
		}
		nics = append(nics, vnicWithIndex{Vnic: response.Vnic, Idx: *val.NicIndex})
	}
	return nics, nil
}

func (o *ociInstance) getAddresses() ([]corenetwork.ProviderAddress, error) {
	vnics, err := o.getVnics()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var addresses []corenetwork.ProviderAddress
	for _, val := range vnics {
		if val.Vnic.PrivateIp != nil {
			privateAddress := corenetwork.ProviderAddress{
				MachineAddress: corenetwork.MachineAddress{
					Value: *val.Vnic.PrivateIp,
					Type:  corenetwork.IPv4Address,
					Scope: corenetwork.ScopeCloudLocal,
				},
			}
			addresses = append(addresses, privateAddress)
		}
		if val.Vnic.PublicIp != nil {
			publicAddress := corenetwork.ProviderAddress{
				MachineAddress: corenetwork.MachineAddress{
					Value: *val.Vnic.PublicIp,
					Type:  corenetwork.IPv4Address,
					Scope: corenetwork.ScopePublic,
				},
			}
			addresses = append(addresses, publicAddress)
		}
	}
	return addresses, nil
}

// Addresses implements instances.Instance
func (o *ociInstance) Addresses(ctx envcontext.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	addresses, err := o.getAddresses()
	ocicommon.HandleCredentialError(err, ctx)
	return addresses, err
}

func (o *ociInstance) isTerminating() bool {
	terminatedStatus := ociCore.InstanceLifecycleStateTerminated
	terminatingStatus := ociCore.InstanceLifecycleStateTerminating
	if o.raw.LifecycleState == terminatedStatus || o.raw.LifecycleState == terminatingStatus {
		return true
	}
	return false
}

func (o *ociInstance) waitForPublicIP(ctx envcontext.ProviderCallContext) error {
	iteration := 0
	startTime := time.Now()
	for {
		addresses, err := o.Addresses(ctx)
		if err != nil {
			ocicommon.HandleCredentialError(err, ctx)
			return errors.Trace(err)
		}
		if iteration >= maxPollIterations {
			logger.Debugf(ctx, "could not find a public IP after %s. breaking loop", time.Since(startTime))
			break
		}

		for _, val := range addresses {
			if val.Scope == corenetwork.ScopePublic {
				logger.Infof(ctx, "Found public IP: %s", val)
				return nil
			}
		}
		<-o.env.clock.After(pollTime)
		iteration++
		continue
	}
	return errors.NotFoundf("failed to find public IP for instance: %s", *o.raw.Id)
}

func (o *ociInstance) deleteInstance(ctx envcontext.ProviderCallContext) error {
	err := o.refresh()
	if errors.Is(err, errors.NotFound) {
		return nil
	}

	if o.isTerminating() {
		logger.Debugf(ctx, "instance %q is alrealy in terminating state", *o.raw.Id)
		return nil
	}
	request := ociCore.TerminateInstanceRequest{
		InstanceId: o.raw.Id,
		IfMatch:    o.etag,
	}
	response, err := o.env.Compute.TerminateInstance(context.Background(), request)
	if err != nil && !o.env.isNotFound(response.RawResponse) {
		ocicommon.HandleCredentialError(err, ctx)
		return err
	}
	iteration := 0
	for {
		if err := o.refresh(); err != nil {
			if errors.Is(err, errors.NotFound) {
				break
			}
			ocicommon.HandleCredentialError(err, ctx)
			return err
		}
		logger.Infof(ctx, "Waiting for machine to transition to Terminating: %s", o.raw.LifecycleState)
		if o.isTerminating() {
			break
		}
		if iteration >= maxPollIterations && o.raw.LifecycleState == ociCore.InstanceLifecycleStateRunning {
			return errors.Errorf("Instance still in running state after %v checks", iteration)
		}
		<-o.env.clock.After(pollTime)
		iteration++
		continue
	}
	// TODO(gsamfira): cleanup firewall rules
	// TODO(gsamfira): cleanup VNIC?
	return nil
}

// hardwareCharacteristics returns the hardware characteristics of the current
// instance
func (o *ociInstance) hardwareCharacteristics() *instance.HardwareCharacteristics {
	shapeCfg := o.raw.ShapeConfig
	var mem, cpus *uint64
	var archType string
	if shapeCfg != nil {
		if shapeCfg.MemoryInGBs != nil {
			v := uint64(*shapeCfg.MemoryInGBs * 1024)
			mem = &v
		}
		if shapeCfg.Ocpus != nil {
			v := uint64(*shapeCfg.Ocpus)
			cpus = &v
		}
		if shapeCfg.ProcessorDescription != nil {
			archType = archTypeByProcessorDescription(*shapeCfg.ProcessorDescription)
		}
	}
	var az string
	if o.raw.AvailabilityDomain != nil {
		az = *o.raw.AvailabilityDomain
	}
	hc := &instance.HardwareCharacteristics{
		Arch:     &archType,
		Mem:      mem,
		CpuCores: cpus,
	}
	if az != "" {
		hc.AvailabilityZone = &az
	}
	return hc
}

func (o *ociInstance) waitForMachineStatus(state ociCore.InstanceLifecycleStateEnum, timeout time.Duration) error {
	timer := o.env.clock.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.Chan():
			return errors.Errorf(
				"Timed out waiting for instance to transition from %v to %v",
				o.raw.LifecycleState, state,
			)
		case <-o.env.clock.After(pollTime):
			err := o.refresh()
			if err != nil {
				return err
			}
			if o.raw.LifecycleState == state {
				return nil
			}
		}
	}
}

func (o *ociInstance) refresh() error {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	request := ociCore.GetInstanceRequest{
		InstanceId: o.raw.Id,
	}
	response, err := o.env.Compute.GetInstance(context.Background(), request)
	if err != nil {
		if response.RawResponse != nil && response.RawResponse.StatusCode == http.StatusNotFound {
			// If we care about 404 errors, this makes it easier to test using
			// errors.IsNotFound
			return errors.NotFoundf("instance %s was not found", *o.raw.Id)
		}
		return err
	}
	o.etag = response.Etag
	o.raw = response.Instance
	return nil
}

// OpenPorts (InstanceFirewaller) ensures that the input ingress rule is
// permitted for machine with the input ID.
func (o *ociInstance) OpenPorts(ctx envcontext.ProviderCallContext, _ string, rules firewall.IngressRules) error {
	client, err := o.getInstanceConfigurator(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(client.ChangeIngressRules("", true, rules))
}

// OpenPorts (InstanceFirewaller) ensures that the input ingress rule is
// restricted for machine with the input ID.
func (o *ociInstance) ClosePorts(ctx envcontext.ProviderCallContext, _ string, rules firewall.IngressRules) error {
	client, err := o.getInstanceConfigurator(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(client.ChangeIngressRules("", false, rules))
}

// IngressRules (InstanceFirewaller) returns the ingress rules that have been
// applied to the input machine ID.
func (o *ociInstance) IngressRules(ctx envcontext.ProviderCallContext, _ string) (firewall.IngressRules, error) {
	client, err := o.getInstanceConfigurator(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	rules, err := client.FindIngressRules()
	return rules, errors.Trace(err)
}

func (o *ociInstance) getInstanceConfigurator(
	ctx envcontext.ProviderCallContext,
) (common.InstanceConfigurator, error) {
	addresses, err := o.Addresses(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Try to find a public address.
	// Different models use different VCNs (and therefore subnets),
	// so the cloud-local IPs are no good if a controller is trying to
	// configure an instance in another model.
	for _, addr := range addresses {
		if addr.Scope == corenetwork.ScopePublic {
			return o.newInstanceConfigurator(addr.Value), nil
		}
	}

	return nil, errors.NotFoundf("public address for instance %q", o.Id())
}
