// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/description"
	"github.com/juju/errors"
	"github.com/juju/juju/core/firewall"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
)

type FirewallRulesExportSuite struct{}

var _ = gc.Suite(&FirewallRulesExportSuite{})

func (f *FirewallRulesExportSuite) TestExportFirewallRules(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	firewallRule0 := f.firewallRule(ctrl, "uuid-4", "ssh", []string{"192.168.1.0/16"})

	rules := []MigrationFirewallRule{
		firewallRule0,
	}
	source := NewMockFirewallRuleSource(ctrl)
	source.EXPECT().AllFirewallRules().Return(rules, nil)

	model := NewMockFirewallRulesModel(ctrl)
	model.EXPECT().AddFirewallRule(description.FirewallRuleArgs{
		ID:               names.NewControllerTag("uuid-4").String(),
		WellKnownService: "ssh",
		WhitelistCIDRs:   []string{"192.168.1.0/16"},
	})

	migration := ExportFirewallRules{}
	err := migration.Execute(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (f *FirewallRulesExportSuite) TestExportFirewallRulesFailsGettingEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	source := NewMockFirewallRuleSource(ctrl)
	source.EXPECT().AllFirewallRules().Return(nil, errors.New("fail"))

	model := NewMockFirewallRulesModel(ctrl)

	migration := ExportFirewallRules{}
	err := migration.Execute(source, model)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (f *FirewallRulesExportSuite) firewallRule(ctrl *gomock.Controller, id string, service firewall.WellKnownServiceType, cidrs []string) *MockMigrationFirewallRule {
	firewallRule := NewMockMigrationFirewallRule(ctrl)
	firewallRule.EXPECT().ID().Return(names.NewControllerTag(id).String())
	firewallRule.EXPECT().WhitelistCIDRs().Return(cidrs)
	firewallRule.EXPECT().WellKnownService().Return(service)
	return firewallRule
}
