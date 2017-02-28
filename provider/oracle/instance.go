// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	jErr "github.com/juju/errors"

	oci "github.com/hoenirvili/go-oracle-cloud/api"
	"github.com/hoenirvili/go-oracle-cloud/response"
	"github.com/pkg/errors"

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
	status          instance.InstanceStatus
	machine         *response.Instance
	client          *oci.Client
	publicAddresses []response.IpAssociation
}

// newInstance returns a new instance.Instance implementation
// for the response.Instance
func newInstance(params *response.Instance, client *oci.Client) (*oracleInstance, error) {
	if params == nil {
		return nil, errors.Errorf("Instance response is nil")
	}

	instance := &oracleInstance{
		name: params.Name,
		status: instance.InstanceStatus{
			Status:  status.Status(params.State),
			Message: "",
		},
		machine: params,
		client:  client,
	}

	return instance, nil
}

// Id returns a provider generated indentifier for the Instance
func (o oracleInstance) Id() instance.Id {
	return instance.Id(o.name)
}

// Status represents the provider specific status for the instance
func (o oracleInstance) Status() instance.InstanceStatus {
	return o.status
}

func (o oracleInstance) getPublicAddresses() ([]response.IpAssociation, error) {
	ipAssoc := []response.IpAssociation{}
	if len(o.publicAddresses) == 0 {
		assoc, err := o.client.AllIpAssociation()
		if err != nil {
			return nil, jErr.Trace(err)
		}
		for _, val := range assoc.Result {
			if o.machine.Vcable_id == val.Vcable {
				ipAssoc = append(ipAssoc, val)
			}
		}
		o.publicAddresses = ipAssoc
	}
	return o.publicAddresses, nil
}

// Addresses returns a list of hostnames or ip addresses
// associated with the instance.
func (o oracleInstance) Addresses() ([]network.Address, error) {
	addresses := []network.Address{}
	ips, err := o.getPublicAddresses()
	if err != nil {
		return nil, jErr.Trace(err)
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
func (o oracleInstance) OpenPorts(machineId string, rules []network.IngressRule) error {
	return nil
}

// ClosePorts closes the given port ranges on the instance, which
// should have been started with the given machine id.
func (o oracleInstance) ClosePorts(machineId string, rules []network.IngressRule) error {
	return nil
}

// IngressRules returns the set of ingress rules for the instance,
// which should have been applied to the given machine id. The
// rules are returned as sorted by network.SortIngressRules().
// It is expected that there be only one ingress rule result for a given
// port range - the rule's SourceCIDRs will contain all applicable source
// address rules for that port range.
func (o oracleInstance) IngressRules(machineId string) ([]network.IngressRule, error) {
	return nil, nil
}
