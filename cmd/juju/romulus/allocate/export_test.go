// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package allocate

import (
	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func NewAllocateCommandForTest(api apiClient, store jujuclient.ClientStore) cmd.Command {
	c := &allocateCommand{api: api}
	c.SetClientStore(store)
	return modelcmd.Wrap(c)
}
