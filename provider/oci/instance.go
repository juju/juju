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
	"github.com/juju/juju/status"

	envcontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"

	ociCore "github.com/oracle/oci-go-sdk/core"
)

const (
	// MinVolumeSizeMB is the minimum size in MB for a volume or boot disk
	MinVolumeSizeMB = 51200

	// MaxVolumeSizeMB is the maximum size in MB for a volume or boot disk
	MaxVolumeSizeMB = 16777216
)

type ociInstance struct {
	arch     string
	instType *instances.InstanceType
	env      *Environ
	mutex    sync.Mutex
	etag     *string
	raw      ociCore.Instance
}

type vnicWithIndex struct {
	Vnic ociCore.Vnic
	Idx  int
}

var _ instance.Instance = (*ociInstance)(nil)

var statusMap map[ociCore.InstanceLifecycleStateEnum]status.Status = map[ociCore.InstanceLifecycleStateEnum]status.Status{
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
	instance := &ociInstance{
		raw:  raw,
		env:  env,
		arch: "amd64",
	}

	return instance, nil
}

// SetInstance sets the raw property of ociInstance{}
// Testing purposes.
func (o *ociInstance) SetInstance(inst ociCore.Instance) {
	o.raw = inst
}

func (o *ociInstance) availabilityZone() string {
	return *o.raw.AvailabilityDomain
}

// Id implements instance.Instance
func (o *ociInstance) Id() instance.Id {
	return instance.Id(*o.raw.Id)
}

// Status implements instance.Instance
func (o *ociInstance) Status(ctx envcontext.ProviderCallContext) instance.InstanceStatus {
	// This should not happen, unless someone bypassed newInstance()
	// and created the ociInstance{} object manually.
	if err := o.refresh(); err != nil {
		return instance.InstanceStatus{}
	}
	state, ok := statusMap[o.raw.LifecycleState]
	if !ok {
		state = status.Unknown
	}
	return instance.InstanceStatus{
		Status:  state,
		Message: strings.ToLower(string(o.raw.LifecycleState)),
	}
}

func (o *ociInstance) getVnics() ([]vnicWithIndex, error) {
	attachmentRequest := ociCore.ListVnicAttachmentsRequest{
		CompartmentId: o.raw.CompartmentId,
		InstanceId:    o.raw.Id,
	}
	attachments, err := o.env.Compute.ListVnicAttachments(context.Background(), attachmentRequest)
	if err != nil {
		return nil, errors.Trace(err)
	}
	nics := []vnicWithIndex{}
	for _, val := range attachments.Items {
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

func (o *ociInstance) getAddresses() ([]network.Address, error) {
	vnics, err := o.getVnics()
	if err != nil {
		return nil, errors.Trace(err)
	}
	addresses := []network.Address{}

	for _, val := range vnics {
		if val.Vnic.PrivateIp != nil {
			privateAddress := network.Address{
				Value: *val.Vnic.PrivateIp,
				Type:  network.IPv4Address,
				Scope: network.ScopeCloudLocal,
			}
			addresses = append(addresses, privateAddress)
		}
		if val.Vnic.PublicIp != nil {
			publicAddress := network.Address{
				Value: *val.Vnic.PublicIp,
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			}
			addresses = append(addresses, publicAddress)
		}
	}
	return addresses, nil
}

// Addresses implements instance.Instance
func (o *ociInstance) Addresses(ctx envcontext.ProviderCallContext) ([]network.Address, error) {
	addresses, err := o.getAddresses()
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
	for {
		addresses, err := o.Addresses(ctx)
		if err != nil {
			return err
		}
		if iteration >= 30 {
			logger.Warningf("Instance still in running state after %v checks. breaking loop", iteration)
			break
		}

		for _, val := range addresses {
			if val.Scope == network.ScopePublic {
				logger.Infof("Found public IP: %s", val)
				return nil
			}
		}
		<-o.env.clock.After(1 * time.Second)
		iteration++
		continue
	}
	return errors.NotFoundf("failed to find public IP for instance: %s", *o.raw.Id)
}

func (o *ociInstance) deleteInstance() error {
	err := o.refresh()
	if errors.IsNotFound(err) {
		return nil
	}

	if o.isTerminating() {
		logger.Debugf("instance %q is alrealy in terminating state", *o.raw.Id)
		return nil
	}
	request := ociCore.TerminateInstanceRequest{
		InstanceId: o.raw.Id,
		IfMatch:    o.etag,
	}
	response, err := o.env.Compute.TerminateInstance(context.Background(), request)
	if err != nil && !o.env.isNotFound(response.RawResponse) {
		return err
	}
	iteration := 0
	for {
		if err := o.refresh(); err != nil {
			if errors.IsNotFound(err) {
				break
			}
			return err
		}
		logger.Infof("Waiting for machine to transition to Terminating: %s", o.raw.LifecycleState)
		if o.isTerminating() {
			break
		}
		if iteration >= 30 && o.raw.LifecycleState == ociCore.InstanceLifecycleStateRunning {
			logger.Warningf("Instance still in running state after %v checks. breaking loop", iteration)
			break
		}
		<-o.env.clock.After(1 * time.Second)
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
	if o.arch == "" {
		return nil
	}

	hc := &instance.HardwareCharacteristics{Arch: &o.arch}
	if o.instType != nil {
		hc.Mem = &o.instType.Mem
		hc.RootDisk = &o.instType.RootDisk
		hc.CpuCores = &o.instType.CpuCores
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
		case <-o.env.clock.After(10 * time.Second):
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
