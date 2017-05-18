// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/juju/api/applicationoffers"
	"github.com/juju/juju/cmd/modelcmd"
)

// ApplicationOffersCommandBase is a base for various cross model commands.
type ApplicationOffersCommandBase struct {
	modelcmd.ControllerCommandBase
}

// NewApplicationOffersAPI returns an application offers api for the root api endpoint
// that the command returns.
func (c *ApplicationOffersCommandBase) NewApplicationOffersAPI() (*applicationoffers.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return applicationoffers.NewClient(root), nil
}
