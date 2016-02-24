// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/juju/charmcmd"
	"github.com/juju/juju/resource/cmd"
)

// TODO(ericsnow) Get rid of fakeCharmCmdBase once csclient.Client grows the methods.

type fakeCharmCmdBase struct {
	*charmcmd.CommandBase
}

func NewFakeCharmCmdBase(base *charmcmd.CommandBase) cmd.CharmCommandBase {
	return &fakeCharmCmdBase{base}
}

// Connect implements cmd.CommandBase.
func (c *fakeCharmCmdBase) Connect() (cmd.CharmResourceLister, error) {
	client, err := c.CommandBase.Connect()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newCharmStoreClient(nil, client), nil
}
