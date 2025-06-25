// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description/v9"
	"github.com/juju/errors"

	"github.com/juju/juju/core/network/firewall"
)

// MigrationFirewallRule represents a state.FirewallRule
// Point of use interface to enable better encapsulation.
type MigrationFirewallRule interface {
	ID() string
	WellKnownService() firewall.WellKnownServiceType
	WhitelistCIDRs() []string
}

// FirewallRuleSource defines an inplace usage for reading all the remote
// entities.
type FirewallRuleSource interface {
	AllFirewallRules() ([]MigrationFirewallRule, error)
}

// FirewallRulesModel defines an inplace usage for adding a remote entity
// to a model.
type FirewallRulesModel interface {
	AddFirewallRule(args description.FirewallRuleArgs) description.FirewallRule
}

// ExportFirewallRules describes a way to execute a migration for exporting
// firewall rules.
type ExportFirewallRules struct{}

// Execute the migration of the remote entities using typed interfaces, to
// ensure we don't loose any type safety.
// This doesn't conform to an interface because go doesn't have generics, but
// when this does arrive this would be an execellent place to use them.
func (ExportFirewallRules) Execute(src FirewallRuleSource, dst FirewallRulesModel) error {
	firewallRules, err := src.AllFirewallRules()
	if err != nil {
		return errors.Trace(err)
	}
	for _, firewallRule := range firewallRules {
		dst.AddFirewallRule(description.FirewallRuleArgs{
			ID:               firewallRule.ID(),
			WellKnownService: string(firewallRule.WellKnownService()),
			WhitelistCIDRs:   firewallRule.WhitelistCIDRs(),
		})
	}
	return nil
}
