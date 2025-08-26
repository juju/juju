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
	"github.com/juju/juju/core/network/firewall"
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
- juju-application-offer

DEPRECATION WARNING: %v
`

const setRuleHelpExamples = `
    juju set-firewall-rule ssh --allowlist 192.168.1.0/16
`

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
	service   firewall.WellKnownServiceType
	allowlist string
	whitelist string

	newAPIFunc func() (SetFirewallRuleAPI, error)
}

// Info implements cmd.Command.
func (c *setFirewallRuleCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "set-firewall-rule",
		Args:     "<service-name>, --allowlist <cidr>[,<cidr>...]",
		Purpose:  setRuleHelpSummary,
		Doc:      fmt.Sprintf(setRuleHelpDetails, deprecationWarning),
		Examples: setRuleHelpExamples,
		SeeAlso: []string{
			"firewall-rules",
		},
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
		c.service = firewall.WellKnownServiceType(args[0])
		if c.allowlist == "" && c.whitelist == "" {
			return errors.New("no allowlist subnets specified")
		}
		if c.allowlist != "" && c.whitelist != "" {
			return errors.New("cannot specify both whitelist and allowlist")
		}
		if args[0] != "ssh" && args[0] != "juju-application-offer" {
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
Firewall rules have been moved to model configuration settings ` + "`ssh-allow`" + ` and
` + "`saas-ingress-allow`" + `. This command is deprecated in favour of
reading/writing directly to these settings.
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

	switch c.service {
	case firewall.SSHRule:
		err = client.ModelSet(map[string]interface{}{config.SSHAllowKey: c.allowlist})
	case firewall.JujuApplicationOfferRule:
		err = client.ModelSet(map[string]interface{}{config.SAASIngressAllowKey: c.allowlist})
	default:
		return errors.NotSupportedf("service %v", c.service)
	}
	return block.ProcessBlockedError(err, block.BlockChange)
}
