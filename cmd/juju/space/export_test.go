// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"github.com/juju/juju/cmd/modelcmd"
)

func (base *SpaceCommandBase) SetAPI(api SpaceAPI) {
	base.api = api
}

func (c *RemoveCommand) Name() string {
	return c.name
}

func (c *ListCommand) ListFormat() string {
	return c.out.Name()
}

func NewSpaceCommandBase(api SpaceAPI) SpaceCommandBase {
	base := SpaceCommandBase{
		ModelCommandBase: modelcmd.ModelCommandBase{},
		IAASOnlyCommand:  nil,
		api:              api,
	}
	return base
}
