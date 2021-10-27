// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/juju/jujuclient"

	"github.com/juju/juju/cmd/modelcmd"
)

var (
	WebbrowserOpen = &webbrowserOpen
)

func NewDashboardCommandForTest(store jujuclient.ClientStore, api ControllerAPI) cmd.Command {
	d := &dashboardCommand{
		newAPIFunc: func() (ControllerAPI, bool, error) {
			return api, false, nil
		},
	}
	d.SetClientStore(store)
	return modelcmd.Wrap(d)
}
