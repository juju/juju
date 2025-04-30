// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package models

import (
	"context"

	"github.com/juju/juju/core/network/firewall"
)

// ModelFirewaller provides model-level firewall functionality
type ModelFirewaller interface {
	// OpenModelPorts opens the given port ranges on the model firewall
	OpenModelPorts(ctx context.Context, rules firewall.IngressRules) error

	// CloseModelPorts Closes the given port ranges on the model firewall
	CloseModelPorts(ctx context.Context, rules firewall.IngressRules) error

	// ModelIngressRules returns the set of ingress rules on the model firewall.
	// The rules are returned as sorted by network.SortIngressRules().
	// It is expected that there be only one ingress rule result for a given
	// port range - the rule's SourceCIDRs will contain all applicable source
	// address rules for that port range.
	// If the model security group doesn't exist, return a NotFound error
	ModelIngressRules(ctx context.Context) (firewall.IngressRules, error)
}
