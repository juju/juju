// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import "github.com/juju/go-oracle-cloud/common"

// SecList a security list is a group of one or more instances
// that you can specify as the destination or source
// in a security rule. Instances within a security list
// can communicate fully with one another on all ports.
// When you attach an instance to a security list, the inbound
// and outbound policies defined in the security list are applicable
// to that instance. All instances within a security list can
// communicate fully with one another on all protocols and ports.
// While creating an instance, if you do not attach it to any
// security list, the instance gets attached automatically to
// the predefined /Compute-identity_domain/default/default
// security list, which has the inbound policy set to DENY
// and the outbound policy set to PERMIT.
type SecList struct {

	// Account shows the default account for your identity domain.
	Account string `json:"account"`

	// A description of the security list.
	Description string `json:"description,omitempty"`

	// Name of the secure list
	Name string `json:"name"`

	// The policy for outbound traffic from the security list.
	// You can specify one of the following values:
	// - deny: Packets are dropped. No response is sent.
	// - reject: Packets are dropped, but a response is sent.
	// - permit(default): Packets are allowed.
	Outbound_cidr_policy common.SecRuleAction `json:"outbound_cidr_policy"`

	// The policy for inbound traffic to the security list.
	// You can specify one of the following values:
	// - deny(default): Packets are dropped. No response is sent.
	// - reject: Packets are dropped, but a response is sent.
	// - permit: Packets are allowed. This policy effectively turns off the firewall for all instances in this security list.
	Policy common.SecRuleAction `json:"policy"`

	// uniform Resource Identifier
	Uri string `json:"uri"`
}

// AllSecLists type that is filled with all the
// security list that are in the account declared
type AllSecLists struct {
	Result []SecList `json:"result,omitempty"`
}
