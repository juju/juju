// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	"context"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
)

// Instance represents the the realization of a machine in state.
type Instance interface {
	// Id returns a provider-generated identifier for the Instance.
	Id() instance.Id

	// Status returns the provider-specific status for the instance.
	Status(context.Context) instance.Status

	// Addresses returns a list of hostnames or ip addresses
	// associated with the instance.
	Addresses(ctx context.Context) (corenetwork.ProviderAddresses, error)
}

// InstanceFirewaller provides instance-level firewall functionality
type InstanceFirewaller interface {
	// OpenPorts opens the given port ranges on the instance, which
	// should have been started with the given machine id.
	OpenPorts(ctx context.Context, machineId string, rules firewall.IngressRules) error

	// ClosePorts closes the given port ranges on the instance, which
	// should have been started with the given machine id.
	ClosePorts(ctx context.Context, machineId string, rules firewall.IngressRules) error

	// IngressRules returns the set of ingress rules for the instance,
	// which should have been applied to the given machine id. The
	// rules are returned as sorted by network.SortIngressRules().
	// It is expected that there be only one ingress rule result for a given
	// port range - the rule's SourceCIDRs will contain all applicable source
	// address rules for that port range.
	IngressRules(ctx context.Context, machineId string) (firewall.IngressRules, error)
}
