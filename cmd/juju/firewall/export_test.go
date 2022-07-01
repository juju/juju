// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"github.com/juju/cmd/v3"

	"github.com/juju/juju/v2/cmd/modelcmd"
	"github.com/juju/juju/v2/jujuclient/jujuclienttesting"
)

func NewListRulesCommandForTest(
	api ListFirewallRulesAPI,
) cmd.Command {
	aCmd := &listFirewallRulesCommand{
		newAPIFunc: func() (ListFirewallRulesAPI, error) {
			return api, nil
		},
	}
	aCmd.SetClientStore(jujuclienttesting.MinimalStore())
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
	aCmd.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(aCmd)
}
