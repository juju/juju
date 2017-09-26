// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import "github.com/juju/errors"

// FirewallRuleArgs holds the parameters for updating
// one or more firewall rules.
type FirewallRuleArgs struct {
	// Args holds the parameters for updating a firewall rule.
	Args []FirewallRule `json:"args"`
}

// ListFirewallRulesResults holds the results of listing firewall rules.
type ListFirewallRulesResults struct {
	// Rules is a list of firewall rules.
	Rules []FirewallRule
}

// FirewallRule is a rule for ingress through a firewall.
type FirewallRule struct {
	// KnownService is the well known service for a firewall rule.
	KnownService KnownServiceValue `json:"known-service"`

	// WhitelistCIDRS is the ist of subnets allowed access.
	WhitelistCIDRS []string `json:"whitelist-cidrs,omitempty"`
}

// KnownServiceValue describes a well known service for which a
// firewall rule can be set up.
type KnownServiceValue string

const (
	// The supported services for firewall rules.
	// If a new service is added here, remember to update the
	// set-firewall-rule command help text.

	// SSHRule is a rule for SSH connections.
	SSHRule KnownServiceValue = "ssh"

	// JujuControllerRule is a rule for connections to the Juju controller.
	JujuControllerRule KnownServiceValue = "juju-controller"

	// JujuApplicationOfferRule is a rule for connections to a Juju offer.
	JujuApplicationOfferRule KnownServiceValue = "juju-application-offer"
)

// Validate returns an error if the service value is not valid.
func (v KnownServiceValue) Validate() error {
	switch v {
	case SSHRule, JujuControllerRule, JujuApplicationOfferRule:
		return nil
	}
	return errors.NotValidf("known service %q", v)
}
