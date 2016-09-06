// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	coremigration "github.com/juju/juju/core/migration"
)

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller) *Client {
	return &Client{base.NewFacadeCaller(caller, "MigrationTarget")}
}

// Client is the client-side API for the MigrationTarget facade. It is
// used by the migrationmaster worker when talking to the target
// controller during a migration.
type Client struct {
	caller base.FacadeCaller
}

func (c *Client) Prechecks(model coremigration.ModelInfo) error {
	args := params.MigrationModelInfo{
		UUID:         model.UUID,
		Name:         model.Name,
		OwnerTag:     model.Owner.String(),
		AgentVersion: model.AgentVersion,
	}
	return c.caller.FacadeCall("Prechecks", args, nil)
}

// Import takes a serialized model and imports it into the target
// controller.
func (c *Client) Import(bytes []byte) error {
	serialized := params.SerializedModel{Bytes: bytes}
	return c.caller.FacadeCall("Import", serialized, nil)
}

// Abort removes all data relating to a previously imported model.
func (c *Client) Abort(modelUUID string) error {
	args := params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()}
	return c.caller.FacadeCall("Abort", args, nil)
}

// Activate marks a migrated model as being ready to use.
func (c *Client) Activate(modelUUID string) error {
	args := params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()}
	return c.caller.FacadeCall("Activate", args, nil)
}
