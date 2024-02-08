// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package setmeterstatus

import (
	"github.com/juju/cmd/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

func NewCommandForTest(apiRoot base.APICallCloser) cmd.Command {
	cmd := &SetMeterStatusCommand{apiRoot: apiRoot}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(cmd)
}
