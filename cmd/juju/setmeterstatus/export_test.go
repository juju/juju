// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package setmeterstatus

import (
	"github.com/juju/cmd/v3"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var NewClient = &newClient

func NewCommandForTest(store jujuclient.ClientStore) cmd.Command {
	cmd := &SetMeterStatusCommand{}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}
