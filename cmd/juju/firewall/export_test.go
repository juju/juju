// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"context"

	"github.com/juju/juju/api/jujuclient/jujuclienttesting"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
)

func NewListRulesCommandForTest(
	api ListFirewallRulesAPI,
) cmd.Command {
	aCmd := &listFirewallRulesCommand{
		newAPIFunc: func(ctx context.Context) (ListFirewallRulesAPI, error) {
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
		newAPIFunc: func(ctx context.Context) (SetFirewallRuleAPI, error) {
			return api, nil
		},
	}
	aCmd.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(aCmd)
}
