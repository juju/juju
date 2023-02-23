// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"io"
	"sort"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/network/firewall"
)

type firewallRule struct {
	KnownService   firewall.WellKnownServiceType `yaml:"known-service" json:"known-service"`
	WhitelistCIDRS []string                      `yaml:"whitelist-subnets,omitempty" json:"whitelist-subnets,omitempty"`
}

type firewallRules []firewallRule

func (o firewallRules) Len() int      { return len(o) }
func (o firewallRules) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o firewallRules) Less(i, j int) bool {
	return o[i].KnownService < o[j].KnownService
}

func formatListTabular(writer io.Writer, value interface{}) error {
	rules, ok := value.([]firewallRule)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", rules, value)
	}
	formatFirewallRulesTabular(writer, firewallRules(rules))
	return nil
}

// formatFirewallRulesTabular returns a tabular summary of firewall rules.
func formatFirewallRulesTabular(writer io.Writer, rules firewallRules) {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

	sort.Sort(rules)

	w.Println("Service", "Whitelist subnets")
	for _, rule := range rules {
		w.Println(rule.KnownService, strings.Join(rule.WhitelistCIDRS, ","))
	}
	tw.Flush()
}
