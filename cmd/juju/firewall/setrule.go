// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/modelconfig"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
)

var setRuleHelpSummary = `
Sets a firewall rule.`[1:]

var setRuleHelpDetails = `
Firewall rules control ingress to a well known services
within a Juju model. A rule consists of the service name
and a allowlist of allowed ingress subnets.
The currently supported services are:
- ssh

DEPRECATION WARNING: %v

Examples:
    juju set-firewall-rule ssh --allowlist 192.168.1.0/16

See also: 
    firewall-rules`

// NewSetFirewallRuleCommand returns a command to set firewall rules.
func NewSetFirewallRuleCommand() cmd.Command {
	cmd := &setFirewallRuleCommand{}
	cmd.newAPIFunc = func() (SetFirewallRuleAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return modelconfig.NewClient(root), nil

	}
	return modelcmd.Wrap(cmd)
}

type setFirewallRuleCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand
	allowlist string
	whitelist string

	newAPIFunc func() (SetFirewallRuleAPI, error)
}

// Info implements cmd.Command.
func (c *setFirewallRuleCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "set-firewall-rule",
		Args:    "<service-name>, --allowlist <cidr>[,<cidr>...]",
		Purpose: setRuleHelpSummary,
		Doc:     fmt.Sprintf(setRuleHelpDetails, deprecationWarning),
	})
}

// SetFlags implements cmd.Command.
func (c *setFirewallRuleCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.allowlist, "allowlist", "", "list of subnets to allowlist")
	f.StringVar(&c.whitelist, "whitelist", "", "")
}

// Init implements cmd.Command.
func (c *setFirewallRuleCommand) Init(args []string) (err error) {
	if len(args) == 1 {
		if c.allowlist == "" && c.whitelist == "" {
			return errors.New("no allowlist subnets specified")
		}
		if c.allowlist != "" && c.whitelist != "" {
			return errors.New("cannot specify both whitelist and allowlist")
		}
		if args[0] != "ssh" {
			return errors.NotSupportedf("service %q", args[0])
		}
		if err := c.validateCIDRS(c.allowlist + c.whitelist); err != nil {
			return errors.Trace(err)
		}
	}
	if len(args) == 0 {
		return errors.New("no well known service specified")
	}
	return cmd.CheckEmpty(args[1:])
}

func (c *setFirewallRuleCommand) validateCIDRS(value string) error {
	rawValues := strings.Split(value, ",")
	for _, cidrStr := range rawValues {
		cidrStr = strings.TrimSpace(cidrStr)
		if _, _, err := net.ParseCIDR(cidrStr); err != nil {
			return errors.NotValidf(cidrStr)
		}
	}
	return nil
}

// SetFirewallRuleAPI defines the API methods that the set firewall rules command uses.
type SetFirewallRuleAPI interface {
	Close() error
	ModelSet(config map[string]interface{}) error
}

var deprecationWarning = `
This command now just sets/reads the "ssh-allowlist" model-config item and
is deprecated in favour of setting/reading that item with "juju model-config"
`[1:]

func (c *setFirewallRuleCommand) Run(ctx *cmd.Context) error {
	if c.whitelist != "" {
		c.allowlist = c.whitelist
		ctx.Warningf("--whitelist is deprecated in favour of --allowlist")
	}
	ctx.Warningf(deprecationWarning)

	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()
	err = client.ModelSet(map[string]interface{}{config.SSHAllowListKey: c.allowlist})
	return block.ProcessBlockedError(err, block.BlockChange)
}
