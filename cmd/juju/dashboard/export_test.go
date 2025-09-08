// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard

import (
	"context"
	"os"

	"github.com/juju/juju/api/jujuclient"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
)

var (
	WebbrowserOpen = &webbrowserOpen
)

func NewDashboardCommandForTest(store jujuclient.ClientStore, api ControllerAPI, signalCh chan os.Signal, sshCmd cmd.Command) cmd.Command {
	d := &dashboardCommand{
		newAPIFunc: func(ctx context.Context) (ControllerAPI, bool, error) {
			return api, false, nil
		},
		signalCh:       signalCh,
		embeddedSSHCmd: sshCmd,
	}
	d.SetClientStore(store)
	return modelcmd.Wrap(d)
}
