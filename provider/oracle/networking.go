// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	oci "github.com/juju/go-oracle-cloud/api"
	ociResponse "github.com/juju/go-oracle-cloud/response"

	"github.com/juju/errors"
	"github.com/juju/juju/environs"
)

var _ environs.NetworkingEnviron = (*OracleEnviron)(nil)

// Only Ubuntu for now. There is no CentOS image in the oracle
// compute marketplace
var ubuntuInterfaceTemplate = `
auto %s
iface %s inet dhcp
`

const (
	// defaultNicName is the default network card name attached by default
	// to every instance. This NIC is used for outbound internet access
	defaultNicName = "eth0"
	// nicPrefix si the default NIC prefix name for any extra NICs attached
	// to instances spawned by juju
	nicPrefix = "eth"
	// interfacesConfigDir default path of interfaces.d directory on Ubuntu machines
	// currently CentOS is not available in the oracle market, and windows needs
	// no extra configuration to bring up additional NICs
	interfacesConfigDir = `/etc/network/interfaces.d`
)

// DeleteMachineVnicSet will delete the machine vNIC set and any ACLs bound to it.
func (o *OracleEnviron) DeleteMachineVnicSet(machineId string) error {
	if err := o.RemoveACLAndRules(machineId); err != nil {
		return errors.Trace(err)
	}
	name := o.client.ComposeName(o.namespace.Value(machineId))
	if err := o.client.DeleteVnicSet(name); err != nil {
		if !oci.IsNotFound(err) {
			return errors.Trace(err)
		}
	}
	return nil
}

func (o *OracleEnviron) ensureVnicSet(machineId string, tags []string) (ociResponse.VnicSet, error) {
	if access, err := o.SupportsSpaces(); err != nil || access == false {
		logger.Warningf("spaces is not supported on this API endpoint. SupportsSpaces returned: %v; %s", access, err)
		return ociResponse.VnicSet{}, nil
	}

	acl, err := o.CreateDefaultACLAndRules(machineId)
	if err != nil {
		return ociResponse.VnicSet{}, errors.Trace(err)
	}
	name := o.client.ComposeName(o.namespace.Value(machineId))
	details, err := o.client.VnicSetDetails(name)
	if err != nil {
		if !oci.IsNotFound(err) {
			return ociResponse.VnicSet{}, errors.Trace(err)
		}
		logger.Debugf("Creating vnic set %q", name)
		vnicSetParams := oci.VnicSetParams{
			AppliedAcls: []string{
				acl.Name,
			},
			Description: "Juju created vnic set",
			Name:        name,
			Tags:        tags,
		}
		details, err := o.client.CreateVnicSet(vnicSetParams)
		if err != nil {
			return ociResponse.VnicSet{}, errors.Trace(err)
		}
		return details, nil
	}
	return details, nil
}
