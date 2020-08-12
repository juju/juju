// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	oci "github.com/juju/go-oracle-cloud/api"
	ociCommon "github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	oraclenetwork "github.com/juju/juju/provider/oracle/network"
)

// oracleInstance implements the instances.Instance interface
type oracleInstance struct {
	// name of the instance, generated after the vm creation
	name string
	// status holds the status of the instance
	status instance.Status
	// machine will hold the raw instance details obtained from
	// the provider
	machine response.Instance
	// arch will hold the architecture information of the instance
	arch     *string
	instType *instances.InstanceType
	mutex    *sync.Mutex
	env      *OracleEnviron
}

// hardwareCharacteristics returns the hardware characteristics of the current
// instance
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

// extractInstanceIDFromMachineName will return the hostname of the machine
// identified by the provider ID. In the Oracle compute cloud the provider
// IDs of the instances has the following format:
// /Compute-tenant_domain/tenant_username/instance_hostname/instance_UUID
func extractInstanceIDFromMachineName(id string) (instance.Id, error) {
	var instId instance.Id
	name := strings.Split(id, "/")
	if len(name) < 4 {
		return instId, errors.Errorf("invalid instance name: %s", id)
	}
	instId = instance.Id(name[3])
	return instId, nil
}

// newInstance returns a new oracleInstance
func newInstance(params response.Instance, env *OracleEnviron) (*oracleInstance, error) {
	if params.Name == "" {
		return nil, errors.New(
			"Instance response does not contain a name",
		)
	}
	name, err := extractInstanceIDFromMachineName(params.Name)
	if err != nil {
		return nil, err
	}
	mutex := &sync.Mutex{}
	instance := &oracleInstance{
		name: string(name),
		status: instance.Status{
			Status:  status.Status(params.State),
			Message: "",
		},
		machine: params,
		mutex:   mutex,
		env:     env,
	}

	return instance, nil
}

// Id is defined on the instances.Instance interface.
func (o *oracleInstance) Id() instance.Id {
	if o.machine.Name != "" {
		name, err := extractInstanceIDFromMachineName(o.machine.Name)
		if err != nil {
			return instance.Id(o.machine.Name)
		}
		return name
	}

	return instance.Id(o.name)
}

// Status is defined on the instances.Instance interface.
func (o *oracleInstance) Status(ctx context.ProviderCallContext) instance.Status {
	return o.status
}

// StorageAttachments returns the storage that was attached in the moment
// of instance creation. This storage cannot be detached dynamically.
// this is also needed if you wish to determine the free disk index
// you can use when attaching a new disk
func (o *oracleInstance) StorageAttachments() []response.Storage {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	return o.machine.Storage_attachments
}

// refresh refreshes the instance raw details from the oracle api
// this method is mutex protected
func (o *oracleInstance) refresh() error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	machine, err := o.env.client.InstanceDetails(o.machine.Name)
	// if the request failed for any reason
	// we should not update the information and
	// let the old one persist
	if err != nil {
		return err
	}

	o.machine = machine
	return nil
}

// waitForMachineStatus will ping the machine status until the timeout
// duration is reached or an error appeared
func (o *oracleInstance) waitForMachineStatus(state ociCommon.InstanceState, timeout time.Duration) error {
	timer := o.env.clock.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.Chan():
			return errors.Errorf(
				"Timed out waiting for instance to transition from %v to %v",
				o.machine.State, state,
			)
		case <-o.env.clock.After(10 * time.Second):
			err := o.refresh()
			if err != nil {
				return err
			}
			if o.machine.State == state {
				return nil
			}
		}
	}
}

// delete will delete the instance and attempt to cleanup any instance related
// resources
func (o *oracleInstance) deleteInstanceAndResources(cleanup bool) error {
	if cleanup {
		err := o.disassociatePublicIps(true)
		if err != nil {
			return err
		}
	}

	if err := o.env.client.DeleteInstance(o.machine.Name); err != nil {
		return errors.Trace(err)
	}

	if cleanup {
		// Wait for instance to be deleted. The oracle API does not allow us to
		// delete a security list if there is still a VM associated with it.
		iteration := 0
		for {
			if instance, err := o.env.client.InstanceDetails(o.machine.Name); !oci.IsNotFound(err) {
				if instance.State == ociCommon.StateError {
					logger.Warningf("Instance %s entered error state", o.machine.Name)
					break
				}
				if iteration >= 30 && instance.State == ociCommon.StateRunning {
					logger.Warningf("Instance still in running state after %v checks. breaking loop", iteration)
					break
				}
				if oci.IsInternalApi(err) {
					logger.Errorf("got internal server error from API: %q", err)
				}
				<-o.env.clock.After(1 * time.Second)
				iteration++
				continue
			}
			logger.Debugf("Machine %v successfully deleted", o.machine.Name)
			break
		}

		//
		// seclist, vnicset, secrules, and acl created with
		// StartInstanceParams.InstanceConfig.MachineId,
		// convert o.Id() to machineId for deletion.
		// o.Id() returns a string in hostname form.
		tag, err := o.env.namespace.MachineTag(string(o.Id()))
		if err != nil {
			return errors.Annotatef(err, "failed to get a machine tag to complete cleanup of instance")
		}
		machineId := tag.Id()

		// the VM association is now gone, now we can delete the
		// machine sec list
		logger.Debugf("deleting seclist for instance: %s", machineId)
		if err := o.env.DeleteMachineSecList(machineId); err != nil {
			logger.Errorf("failed to delete seclist: %s", err)
			if !oci.IsMethodNotAllowed(err) {
				return errors.Trace(err)
			}
		}
		logger.Debugf("deleting vnic set for instance: %s", machineId)
		if err := o.env.DeleteMachineVnicSet(machineId); err != nil {
			logger.Errorf("failed to delete vnic set: %s", err)
			if !oci.IsMethodNotAllowed(err) {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

// unusedPublicIps returns a slice of IpReservation that are currently not used
func (o *oracleInstance) unusedPublicIps() ([]response.IpReservation, error) {
	filter := []oci.Filter{
		{
			Arg:   "permanent",
			Value: "true",
		},
		{
			Arg:   "used",
			Value: "false",
		},
	}

	res, err := o.env.client.AllIpReservations(filter)
	if err != nil {
		return nil, err
	}

	return res.Result, nil
}

// associatePublicIP associates a public IP with the current instance
func (o *oracleInstance) associatePublicIP() error {
	// return all unused public IPs
	unusedIps, err := o.unusedPublicIps()
	if err != nil {
		return err
	}

	for _, val := range unusedIps {
		assocPoolName := ociCommon.NewIPPool(
			ociCommon.IPPool(val.Name),
			ociCommon.IPReservationType,
		)
		// create the association for it
		if _, err := o.env.client.CreateIpAssociation(
			assocPoolName,
			o.machine.Vcable_id,
		); err != nil {
			if oci.IsBadRequest(err) {
				// the IP probably got allocated after we fetched it
				// from the API. Move on to the next one.
				continue
			}

			return err
		} else {
			if _, err = o.env.client.UpdateIpReservation(val.Name, "", val.Parentpool, val.Permanent, o.machine.Tags); err != nil {
				// we don't really want to terminate execution if we fail to update
				// tags
				logger.Errorf("failed to update IP reservation tags: %q", err)
			}
			return nil
		}
	}

	// no unused IP reservations found. Allocate a new one.
	reservation, err := o.env.client.CreateIpReservation(
		o.machine.Name, ociCommon.PublicIPPool, true, o.machine.Tags)
	if err != nil {
		return err
	}

	// compose IP pool name
	assocPoolName := ociCommon.NewIPPool(
		ociCommon.IPPool(reservation.Name),
		ociCommon.IPReservationType,
	)
	if _, err := o.env.client.CreateIpAssociation(
		assocPoolName,
		o.machine.Vcable_id,
	); err != nil {
		return err
	}

	return nil
}

// dissasociatePublicIps disassociates the public IP address from the current instance.
// Optionally, the remove flag will also remove the IP reservation after the IP was disassociated
func (o *oracleInstance) disassociatePublicIps(remove bool) error {
	associations, err := o.publicAddressesAssociations()
	if err != nil {
		return err
	}

	for _, ipAssoc := range associations {
		reservation := ipAssoc.Reservation
		name := ipAssoc.Name
		if err := o.env.client.DeleteIpAssociation(name); err != nil {
			if oci.IsNotFound(err) {
				continue
			}

			return err
		}

		if remove {
			if err := o.env.client.DeleteIpReservation(reservation); err != nil {
				if oci.IsNotFound(err) {
					return nil
				}
				return err
			}
		}
	}

	return nil
}

// publicAddressesAssociations returns a slice of all IP associations for the current instance
func (o *oracleInstance) publicAddressesAssociations() ([]response.IpAssociation, error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	filter := []oci.Filter{
		{
			Arg:   "vcable",
			Value: string(o.machine.Vcable_id),
		},
	}

	assoc, err := o.env.client.AllIpAssociations(filter)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return assoc.Result, nil
}

// Addresses is defined on the instances.Instance interface.
func (o *oracleInstance) Addresses(ctx context.ProviderCallContext) (network.ProviderAddresses, error) {
	var addresses []network.ProviderAddress

	ips, err := o.publicAddressesAssociations()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(o.machine.Attributes.Network) > 0 {
		for name, val := range o.machine.Attributes.Network {
			if _, ip, err := oraclenetwork.GetMacAndIP(val.Address); err == nil {
				address := network.NewScopedProviderAddress(ip, network.ScopeCloudLocal)
				addresses = append(addresses, address)
			} else {
				logger.Errorf("failed to get IP address for NIC %q: %q", name, err)
			}
		}
	}

	for _, val := range ips {
		address := network.NewScopedProviderAddress(val.Ip, network.ScopePublic)
		addresses = append(addresses, address)
	}

	return addresses, nil
}

// OpenPorts is defined on the instances.Instance interface.
func (o *oracleInstance) OpenPorts(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	if o.env.Config().FirewallMode() != config.FwInstance {
		return errors.Errorf(
			"invalid firewall mode %q for opening ports on instance",
			o.env.Config().FirewallMode(),
		)
	}

	return o.env.OpenPortsOnInstance(ctx, machineId, rules)
}

// ClosePorts is defined on the instances.Instance interface.
func (o *oracleInstance) ClosePorts(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	if o.env.Config().FirewallMode() != config.FwInstance {
		return errors.Errorf(
			"invalid firewall mode %q for closing ports on instance",
			o.env.Config().FirewallMode(),
		)
	}

	return o.env.ClosePortsOnInstance(ctx, machineId, rules)
}

// IngressRules is defined on the instances.Instance interface.
func (o *oracleInstance) IngressRules(ctx context.ProviderCallContext, machineId string) (firewall.IngressRules, error) {
	return o.env.MachineIngressRules(ctx, machineId)
}
