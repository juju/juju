// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/firewallrules"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

var listRulesHelpSummary = `
Prints the firewall rules.`[1:]

var listRulesHelpDetails = `
Lists the firewall rules which control ingress to well known services
within a Juju model.

Examples:
    juju list-firewall-rules
    juju firewall-rules

See also: 
    set-firewall-rule`

// NewListFirewallRulesCommand returns a command to list firewall rules.
func NewListFirewallRulesCommand() cmd.Command {
	cmd := &listFirewallRulesCommand{}
	cmd.newAPIFunc = func() (ListFirewallRulesAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return firewallrules.NewClient(root), nil

	}
	return modelcmd.Wrap(cmd)
}

type listFirewallRulesCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output

	newAPIFunc func() (ListFirewallRulesAPI, error)
}

// Info implements cmd.Command.
func (c *listFirewallRulesCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-firewall-rules",
		Purpose: listRulesHelpSummary,
		Doc:     listRulesHelpDetails,
		Aliases: []string{"firewall-rules"},
	}
}

// SetFlags implements cmd.Command.
func (c *listFirewallRulesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatListTabular,
	})
}

// Init implements cmd.Command.
func (c *listFirewallRulesCommand) Init(args []string) (err error) {
	return cmd.CheckEmpty(args)
}

// ListFirewallRulesAPI defines the API methods that the list firewall rules command uses.
type ListFirewallRulesAPI interface {
	Close() error
	ListFirewallRules() ([]params.FirewallRule, error)
}

// Run implements cmd.Command.
func (c *listFirewallRulesCommand) Run(ctx *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()
	rulesResult, err := client.ListFirewallRules()
	if err != nil {
		return err
	}

	rules := make([]firewallRule, len(rulesResult))
	for i, r := range rulesResult {
		rules[i] = firewallRule{
			KnownService:   string(r.KnownService),
			WhitelistCIDRS: r.WhitelistCIDRS,
		}
	}
	return c.out.Write(ctx, rules)
}
