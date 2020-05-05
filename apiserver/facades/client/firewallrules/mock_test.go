// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewallrules_test

import (
	"github.com/juju/names/v4"
	jtesting "github.com/juju/testing"

	"github.com/juju/juju/apiserver/facades/client/firewallrules"
	"github.com/juju/juju/core/firewall"
	"github.com/juju/juju/state"
)

type mockBackend struct {
	jtesting.Stub
	firewallrules.Backend

	modelUUID string
	rules     map[string]state.FirewallRule
}

func (m *mockBackend) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	return nil, false, nil
}

func (m *mockBackend) ModelTag() names.ModelTag {
	m.MethodCall(m, "ModelTag")
	m.PopNoErr()
	return names.NewModelTag(m.modelUUID)
}

func (m *mockBackend) SaveFirewallRule(rule state.FirewallRule) error {
	m.MethodCall(m, "SaveFirewallRule")
	m.PopNoErr()
	m.rules[string(rule.WellKnownService())] = rule
	return nil
}

func (m *mockBackend) ListFirewallRules() ([]*state.FirewallRule, error) {
	m.MethodCall(m, "ListFirewallRules")
	m.PopNoErr()
	frls := make([]*state.FirewallRule, 1)
	firewareRule := state.NewFirewallRule(firewall.JujuApplicationOfferRule, []string{"1.2.3.4/8"})
	frls[0] = &firewareRule
	return frls, nil
}

type mockBlockChecker struct {
	jtesting.Stub
}

func (c *mockBlockChecker) ChangeAllowed() error {
	c.MethodCall(c, "ChangeAllowed")
	return c.NextErr()
}
