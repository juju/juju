// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

func (base *SpaceCommandBase) SetAPI(api SpaceAPI) {
	base.api = api
}

func (c *RemoveCommand) Name() string {
	return c.name
}

func (c *ListCommand) ListFormat() string {
	return c.out.Name()
}
