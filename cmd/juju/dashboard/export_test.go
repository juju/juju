// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard

import (
	"github.com/juju/cmd/v3"

	"github.com/juju/juju/cmd/modelcmd"
)

var (
	ClientGet      = &clientGet
	WebbrowserOpen = &webbrowserOpen
)

func NewDashboardCommandForTest() cmd.Command {
	return modelcmd.Wrap(&dashboardCommand{})
}
