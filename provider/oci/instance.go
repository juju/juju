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

	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"

	ociCore "github.com/oracle/oci-go-sdk/core"
)

const (
	// RootDiskSize is the size of the root disk for all instances deployed on OCI
	// This is not configurable. At the time of this writing, there is no way to
	// request a root disk of a different size.
	RootDiskSize = 51200
)

type ociInstance struct {
	arch     *string
	instType *instances.InstanceType
	env      *Environ
	mutex    *sync.Mutex
	ocid     string
	etag     *string
	raw      ociCore.Instance
}

var _ instance.Instance = (*ociInstance)(nil)
var _ instance.InstanceFirewaller = (*ociInstance)(nil)

// newInstance returns a new oracleInstance
func newInstance(raw ociCore.Instance, env *Environ) (*ociInstance, error) {
	if raw.Id == nil {
		return nil, errors.New(
			"Instance response does not contain an ID",
		)
	}
	mutex := &sync.Mutex{}
	arch := "amd64"
	instance := &ociInstance{
		raw:   raw,
		mutex: mutex,
		env:   env,
		arch:  &arch,
		ocid:  *raw.Id,
	}

	return instance, nil
}

func (o *ociInstance) availabilityZone() string {
	return *o.raw.AvailabilityDomain
}

// Id implements instance.Instance
func (o *ociInstance) Id() instance.Id {
	if o.raw.Id == nil {
		return ""
	}
	return instance.Id(*o.raw.Id)
}

// Status implements instance.Instance
func (o *ociInstance) Status() instance.InstanceStatus {
	if o.raw.Id == nil {
		if err := o.refresh(); err != nil {
			return instance.InstanceStatus{}
		}
	}
	return instance.InstanceStatus{
		Status:  status.Status(o.raw.LifecycleState),
		Message: strings.ToLower(string(o.raw.LifecycleState)),
	}
}

// Addresses implements instance.Instance
func (o *ociInstance) Addresses() ([]network.Address, error) {
	addresses, err := o.env.cli.GetInstanceAddresses(o.Id(), o.raw.CompartmentId)
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

func (o *ociInstance) waitForPublicIP() error {
	iteration := 0
	for {
		addresses, err := o.Addresses()
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
	return errors.NotFoundf("failed to find public IP for instance: %s", o.raw.Id)
}

func (o *ociInstance) deleteInstance() error {
	err := o.refresh()
	if errors.IsNotFound(err) {
		return nil
	}

	if o.isTerminating() {
		return nil
	}
	request := ociCore.TerminateInstanceRequest{
		InstanceId: &o.ocid,
		IfMatch:    o.etag,
	}
	response, err := o.env.cli.TerminateInstance(context.Background(), request)
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

// OpenPorts implements instance.InstanceFirewaller
func (o *ociInstance) OpenPorts(machineId string, rules []network.IngressRule) error {
	return nil
}

// ClosePorts implements instance.InstanceFirewaller
func (o *ociInstance) ClosePorts(machineId string, rules []network.IngressRule) error {
	return nil
}

// IngressRules implements instance.InstanceFirewaller
func (o *ociInstance) IngressRules(machineId string) ([]network.IngressRule, error) {
	return nil, nil
}

// hardwareCharacteristics returns the hardware characteristics of the current
// instance
func (o *ociInstance) hardwareCharacteristics() *instance.HardwareCharacteristics {
	if o.arch == nil {
		return nil
	}

	hc := &instance.HardwareCharacteristics{Arch: o.arch}
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
		InstanceId: &o.ocid,
	}
	response, err := o.env.cli.GetInstance(context.Background(), request)
	if err != nil {
		if response.RawResponse != nil && response.RawResponse.StatusCode == http.StatusNotFound {
			// If we care about 404 errors, this makes it easier to test using
			// errors.IsNotFound
			return errors.NotFoundf("instance %s was not found", o.ocid)
		}
		return err
	}
	o.etag = response.Etag
	o.raw = response.Instance
	return nil
}
