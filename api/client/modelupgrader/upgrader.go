// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/juju/juju/v3/api/base"
	"github.com/juju/juju/v3/rpc/params"
)

// Client provides methods that the Juju client command uses to upgrade models.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "ModelUpgrader")
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
	}
}

// AbortModelUpgrade aborts and archives the model upgrade
// synchronisation record, if any.
func (c *Client) AbortModelUpgrade(modelUUID string) error {
	args := params.ModelParam{
		ModelTag: names.NewModelTag(modelUUID).String(),
	}
	return c.facade.FacadeCall("AbortModelUpgrade", args, nil)
}

// UpgradeModel upgrades the model to the provided agent version.
func (c *Client) UpgradeModel(
	modelUUID string, version version.Number, stream string, ignoreAgentVersions, druRun bool,
) error {
	args := params.UpgradeModel{
		ModelTag:            names.NewModelTag(modelUUID).String(),
		ToVersion:           version,
		AgentStream:         stream,
		IgnoreAgentVersions: ignoreAgentVersions,
		DryRun:              druRun,
	}
	return c.facade.FacadeCall("UpgradeModel", args, nil)
}
