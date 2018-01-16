// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewallrules

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client allows access to the firewall rules API end point.
type Client struct {
	base.ClientFacade
	st     base.APICallCloser
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the firewall rules api.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "FirewallRules")
	return &Client{ClientFacade: frontend, st: st, facade: backend}
}

// SetFirewallRule creates or updates a firewall rule.
func (c *Client) SetFirewallRule(service string, whiteListCidrs []string) error {
	serviceValue := params.KnownServiceValue(service)
	if err := serviceValue.Validate(); err != nil {
		return errors.Trace(err)
	}

	args := params.FirewallRuleArgs{
		Args: []params.FirewallRule{
			{
				KnownService:   serviceValue,
				WhitelistCIDRS: whiteListCidrs,
			}},
	}
	var results params.ErrorResults
	if err := c.facade.FacadeCall("SetFirewallRules", args, &results); err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// ListFirewallRules returns all the firewall rules.
func (c *Client) ListFirewallRules() ([]params.FirewallRule, error) {
	var results params.ListFirewallRulesResults
	if err := c.facade.FacadeCall("ListFirewallRules", nil, &results); err != nil {
		return nil, errors.Trace(err)
	}
	return results.Rules, nil
}
