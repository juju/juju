// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/juju/state/api/params"
)

// Actions returns the available actions
func (c *Client) Actions(args params.Actions) (params.ActionsResult, error) {
	unit, err := c.api.state.Unit(args.UnitName)
	if err != nil {
		return params.ActionsResult{}, err
	}

	service, err := unit.Service()
	if err != nil {
		return params.ActionsResult{}, err
	}

	charm, _, err := service.Charm()
	if err != nil {
		return params.ActionsResult{}, err
	}

	actionspec := charm.Actions()

	actions := params.ActionInfo{}

	for spec := range actionspec.ActionSpecs {
		actions[spec] = actionspec.ActionSpecs[spec].Description
	}

	return params.ActionsResult{Actions: actions}, nil
}
