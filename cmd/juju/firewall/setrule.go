// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"fmt"

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
and a whitelist of allowed ingress subnets.
Only ssh is supported

DEPRECATION WARNING: %v

Examples:
    juju set-firewall-rule ssh --whitelist 192.168.1.0/16

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
	whitelist string

	newAPIFunc func() (SetFirewallRuleAPI, error)
}

// Info implements cmd.Command.
func (c *setFirewallRuleCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "set-firewall-rule",
		Args:    "<service-name>, --whitelist <cidr>[,<cidr>...]",
		Purpose: setRuleHelpSummary,
		Doc:     fmt.Sprintf(setRuleHelpDetails, deprecationWarning),
	})
}

// SetFlags implements cmd.Command.
func (c *setFirewallRuleCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.whitelist, "whitelist", "", "list of subnets to whitelist")
}

// Init implements cmd.Command.
func (c *setFirewallRuleCommand) Init(args []string) (err error) {
	if len(args) == 1 {
		if c.whitelist == "" {
			return errors.New("no whitelist subnets specified")
		}
		if args[0] != "ssh" {
			return errors.NotSupportedf("service %q", args[0])
		}
	}
	if len(args) == 0 {
		return errors.New("no well known service specified")
	}
	return cmd.CheckEmpty(args[1:])
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
	ctx.Warningf(deprecationWarning)

	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()
	err = client.ModelSet(map[string]interface{}{config.SSHAllowListKey: c.whitelist})
	return block.ProcessBlockedError(err, block.BlockChange)
}
