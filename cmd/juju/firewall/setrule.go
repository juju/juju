// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"net"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"fmt"
	"github.com/juju/juju/api/firewallrules"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var setRuleHelpSummary = `
Sets a firewall rule.`[1:]

var setRuleHelpDetails = `
Firewall rules control ingress to a well known services
within a Juju model. A rule consists of the service name
and a usually whitelist of allowed ingress subnets. Some
services also support blacklisting subnets.
The currently supported services are:
%v

Examples:
    juju set-firewall-rule ssh --whitelist 192.168.1.0/16
    juju set-firewall-rule juju-controller --whitelist 192.168.1.0/16 --blacklist 10.0.0.0/8,192.168.2.0/8

See also: 
    list-firewall-rules`

// NewSetFirewallRuleCommand returns a command to set firewall rules.
func NewSetFirewallRuleCommand() cmd.Command {
	cmd := &setFirewallRuleCommand{}
	cmd.newAPIFunc = func() (SetFirewallRuleAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return firewallrules.NewClient(root), nil

	}
	return modelcmd.Wrap(cmd)
}

type setFirewallRuleCommand struct {
	modelcmd.ModelCommandBase
	service        string
	whitelistValue string
	blacklistValue string

	whiteList  []string
	blackList  []string
	newAPIFunc func() (SetFirewallRuleAPI, error)
}

// Info implements cmd.Command.
func (c *setFirewallRuleCommand) Info() *cmd.Info {
	supportedRules := []string{
		" -" + string(params.SSHRule),
		" -" + string(params.JujuControllerRule),
		" -" + string(params.JujuApplicationOfferRule),
	}
	return &cmd.Info{
		Name:    "set-firewall-rule",
		Args:    "<service-name>, [--whitelist <cidr>,...], [--blacklist <cidr>,...]",
		Purpose: setRuleHelpSummary,
		Doc:     fmt.Sprintf(setRuleHelpDetails, strings.Join(supportedRules, "\n")),
	}
}

// SetFlags implements cmd.Command.
func (c *setFirewallRuleCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.whitelistValue, "whitelist", "", "list of subnets to whitelist")
	f.StringVar(&c.blacklistValue, "blacklist", "", "list of subnets to blacklist")
}

// Init implements cmd.Command.
func (c *setFirewallRuleCommand) Init(args []string) (err error) {
	if len(args) == 1 {
		c.service = args[0]
		if err := c.parseCIDRs(&c.whiteList, c.whitelistValue); err != nil {
			return errors.Annotate(err, "invalid white-list subnet")
		}
		if err := c.parseCIDRs(&c.blackList, c.blacklistValue); err != nil {
			return errors.Annotate(err, "invalid black-list subnet")
		}
		return nil
	}
	if len(args) == 0 {
		return errors.New("no well known service specified")
	}
	return cmd.CheckEmpty(args[1:])
}

func (c *setFirewallRuleCommand) parseCIDRs(cidrs *[]string, value string) error {
	if value == "" {
		return nil
	}
	rawValues := strings.Split(value, ",")
	for _, cidrStr := range rawValues {
		cidrStr = strings.TrimSpace(cidrStr)
		if _, _, err := net.ParseCIDR(cidrStr); err != nil {
			return err
		}
		*cidrs = append(*cidrs, cidrStr)
	}
	return nil
}

// SetFirewallRuleAPI defines the API methods that the set firewall rules command uses.
type SetFirewallRuleAPI interface {
	Close() error
	SetFirewallRule(service string, whiteListCidrs, blackListCidrs []string) error
}

func (c *setFirewallRuleCommand) Run(_ *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()
	err = client.SetFirewallRule(c.service, c.whiteList, c.blackList)
	return block.ProcessBlockedError(err, block.BlockChange)
}
