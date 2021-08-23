// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package budget

import (
	"github.com/juju/cmd/v3"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func NewBudgetCommandForTest(api apiClient, store jujuclient.ClientStore) cmd.Command {
	c := &budgetCommand{api: api}
	c.SetClientStore(store)
	return modelcmd.Wrap(c)
}
