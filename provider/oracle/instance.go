// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"strings"
	"sync"
	"time"

	jujuErr "github.com/juju/errors"

	oci "github.com/hoenirvili/go-oracle-cloud/api"
	ociCommon "github.com/hoenirvili/go-oracle-cloud/common"
	"github.com/hoenirvili/go-oracle-cloud/response"
	ociResponse "github.com/hoenirvili/go-oracle-cloud/response"
	"github.com/pkg/errors"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
)

// oracleInstance represents the realization of amachine instate
// instance imlements the instance.Instance interface
type oracleInstance struct {
	// name of the instance, generated after the vm creation
	name string
	// status represents the status for a provider instance
	status    instance.InstanceStatus
	machine   response.Instance
	client    *oci.Client
	arch      *string
	instType  *instances.InstanceType
	mutex     sync.Mutex
	env       *oracleEnviron
	machineId string
}

func (o *oracleInstance) hardwareCharacteristics() *instance.HardwareCharacteristics {
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

// newInstance returns a new instance.Instance implementation
// for the response.Instance
func newInstance(params response.Instance, env *oracleEnviron) (*oracleInstance, error) {
	if params.Name == "" {
		return nil, errors.Errorf("Instance response does not contain a name")
	}
	//gsamfira: there must be a better way to do this.
	splitMachineName := strings.Split(params.Label, "-")
	machineId := splitMachineName[len(splitMachineName)-1]
	mutex := sync.Mutex{}
	instance := &oracleInstance{
		name: params.Name,
		status: instance.InstanceStatus{
			Status:  status.Status(params.State),
			Message: "",
		},
		machine:   params,
		client:    env.client,
		mutex:     mutex,
		env:       env,
		machineId: machineId,
	}

	return instance, nil
}

// Id returns a provider generated indentifier for the Instance
func (o *oracleInstance) Id() instance.Id {
	if o.machine.Name != "" {
		return instance.Id(o.machine.Name)
	}
	return instance.Id(o.name)
}

// Status represents the provider specific status for the instance
func (o *oracleInstance) Status() instance.InstanceStatus {
	return o.status
}

func (o *oracleInstance) refresh() error {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	machine, err := o.client.InstanceDetails(o.name)
	if err != nil {
		return err
	}
	o.machine = machine
	return nil
}

func (o *oracleInstance) waitForMachineStatus(state ociCommon.InstanceState, timeout time.Duration) error {
	errChan := make(chan error)
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				err := o.refresh()
				if err != nil {
					errChan <- err
					return
				}
				if o.machine.State == ociCommon.StateError {
					errChan <- errors.Errorf("Machine %v entered error state", o.machine.Name)
					return
				}
				if o.machine.State == state {
					errChan <- nil
					return
				}
				time.Sleep(1 * time.Second)
			}
		}
	}()
	select {
	case err := <-errChan:
		return err
	case <-time.After(timeout):
		done <- true
		return errors.Errorf("Timed out waiting for instance to transition from %v to %v", o.machine.State, state)
	}
	return nil
}

func (o *oracleInstance) delete(cleanup bool) error {
	if cleanup {
		err := o.disassociatePublicIps(true)
		if err != nil {
			return err
		}
	}
	err := o.client.DeleteInstance(o.name)
	if err != nil {
		return jujuErr.Trace(err)
	}
	if cleanup {
		// Wait for instance to be deleted. The oracle API does not allow us to
		// delete a security list if there is still a VM associated with it.
		iteration := 0
		for {
			if instance, err := o.client.InstanceDetails(o.name); !oci.IsNotFound(err) {
				if instance.State == ociCommon.StateError {
					logger.Warningf("Instance %s entered error state", o.name)
					break
				}
				if iteration >= 30 && instance.State == ociCommon.StateRunning {
					logger.Warningf("Instance still in running state after %q checks. breaking loop", iteration)
					break
				}
				time.Sleep(1 * time.Second)
				iteration++
				continue
			}
			logger.Debugf("Machine %v successfully deleted", o.name)
			break
		}
		err = o.env.fw.DeleteMachineSecList(o.machineId)
		if err != nil {
			return jujuErr.Trace(err)
		}
	}
	return nil
}

func (o *oracleInstance) deletePublicIps() error {
	ipAssoc, err := o.getPublicAddresses()
	if err != nil {
		return err
	}

	for _, ip := range ipAssoc {
		if err := o.client.DeleteIpReservation(ip.Reservation); err != nil {
			logger.Errorf("Failed to delete IP: %s", err)
			if oci.IsNotFound(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func (o *oracleInstance) getUnusedPublicIps() ([]ociResponse.IpReservation, error) {
	filter := []oci.Filter{
		oci.Filter{
			Arg:   "permanent",
			Value: "true",
		},
		oci.Filter{
			Arg:   "used",
			Value: "false",
		},
	}

	res, err := o.client.AllIpReservations(filter)
	if err != nil {
		return nil, err
	}
	return res.Result, nil
}

func (o *oracleInstance) associatePublicIP() error {
	unusedIps, err := o.getUnusedPublicIps()
	if err != nil {
		return err
	}
	for _, val := range unusedIps {
		assocPoolName := ociCommon.NewIPPool(ociCommon.IPPool(val.Name), ociCommon.IPReservationType)
		if _, err := o.client.CreateIpAssociation(assocPoolName, o.machine.Vcable_id); err != nil {
			if oci.IsBadRequest(err) {
				//The IP probably got allocated after we fetched it
				//from the API. Move on to the next one.
				continue
			}
			return err
		} else {
			//TODO(gsamfira): update IP tags
			return nil
		}
	}
	//no unused IP reservations found. Allocate a new one.
	reservation, err := o.client.CreateIpReservation(
		o.machine.Name, "", ociCommon.PublicIPPool, true, o.machine.Tags)
	if err != nil {
		return err
	}
	assocPoolName := ociCommon.NewIPPool(ociCommon.IPPool(reservation.Name), ociCommon.IPReservationType)
	if _, err := o.client.CreateIpAssociation(assocPoolName, o.machine.Vcable_id); err != nil {
		return err
	}
	return nil
}

func (o *oracleInstance) disassociatePublicIps(remove bool) error {
	publicIps, err := o.getPublicAddresses()
	if err != nil {
		return err
	}
	for _, ipAssoc := range publicIps {
		reservation := ipAssoc.Reservation
		name := ipAssoc.Name
		if err := o.client.DeleteIpAssociation(name); err != nil {
			if oci.IsNotFound(err) {
				continue
			}
			return err
		}
		if remove {
			if err := o.client.DeleteIpReservation(reservation); err != nil {
				if oci.IsNotFound(err) {
					return nil
				}
				return err
			}
		}
	}
	return nil
}

func (o *oracleInstance) getPublicAddresses() ([]response.IpAssociation, error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	ipAssoc := []response.IpAssociation{}
	filter := []oci.Filter{
		oci.Filter{
			Arg:   "vcable",
			Value: string(o.machine.Vcable_id),
		},
	}
	assoc, err := o.client.AllIpAssociations(filter)
	if err != nil {
		return nil, jujuErr.Trace(err)
	}
	for _, val := range assoc.Result {
		ipAssoc = append(ipAssoc, val)
	}
	return ipAssoc, nil
}

// Addresses returns a list of hostnames or ip addresses
// associated with the instance.
func (o *oracleInstance) Addresses() ([]network.Address, error) {
	//TODO (gsamfira): also include addresses on vNics
	addresses := []network.Address{}
	ips, err := o.getPublicAddresses()
	if err != nil {
		return nil, jujuErr.Trace(err)
	}
	if o.machine.Ip != "" {
		address := network.NewScopedAddress(o.machine.Ip, network.ScopeCloudLocal)
		addresses = append(addresses, address)
	}
	for _, val := range ips {
		address := network.NewScopedAddress(val.Ip, network.ScopePublic)
		addresses = append(addresses, address)
	}
	return addresses, nil
}

// OpenPorts opens the given port ranges on the instance, which
// should have been started with the given machine id.
func (o *oracleInstance) OpenPorts(machineId string, rules []network.IngressRule) error {
	if o.env.Config().FirewallMode() != config.FwInstance {
		return errors.Errorf("invalid firewall mode %q for opening ports on instance", o.env.Config().FirewallMode())
	}
	return o.env.fw.OpenPortsOnInstance(machineId, rules)
}

// ClosePorts closes the given port ranges on the instance, which
// should have been started with the given machine id.
func (o *oracleInstance) ClosePorts(machineId string, rules []network.IngressRule) error {
	if o.env.Config().FirewallMode() != config.FwInstance {
		return errors.Errorf("invalid firewall mode %q for closing ports on instance", o.env.Config().FirewallMode())
	}
	return o.env.fw.ClosePortsOnInstance(machineId, rules)
}

// IngressRules returns the set of ingress rules for the instance,
// which should have been applied to the given machine id. The
// rules are returned as sorted by network.SortIngressRules().
// It is expected that there be only one ingress rule result for a given
// port range - the rule's SourceCIDRs will contain all applicable source
// address rules for that port range.
func (o *oracleInstance) IngressRules(machineId string) ([]network.IngressRule, error) {
	return o.env.fw.MachineIngressRules(machineId)
}
