// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
)

func NewListRulesCommandForTest(
	api ListFirewallRulesAPI,
) cmd.Command {
	aCmd := &listFirewallRulesCommand{
		newAPIFunc: func() (ListFirewallRulesAPI, error) {
			return api, nil
		},
	}
	return modelcmd.Wrap(aCmd)
}

func NewSetRulesCommandForTest(
	api SetFirewallRuleAPI,
) cmd.Command {
	aCmd := &setFirewallRuleCommand{
		newAPIFunc: func() (SetFirewallRuleAPI, error) {
			return api, nil
		},
	}
	return modelcmd.Wrap(aCmd)
}
