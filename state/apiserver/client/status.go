// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

func (c *Client) Status(args params.StatusParams) (api.Status, error) {
	conn, err := juju.NewConnFromState(c.api.state)
	if err != nil {
		return api.Status{}, err
	}

	status, err := statecmd.Status(conn, args.Patterns)
	return *status, err
}
