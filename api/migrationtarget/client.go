// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client describes the client side API for the MigrationTarget
// facade. It is called by the migration master worker to talk to the
// target controller during a migration.
type Client interface {
	// Import takes a serialized model and imports it into the target
	// controller.
	Import([]byte) error

	// Abort removes all data relating to a previously imported
	// model.
	Abort(string) error

	// Activate marks a migrated model as being ready to use.
	Activate(string) error
}

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller) Client {
	return &client{base.NewFacadeCaller(caller, "MigrationTarget")}
}

// client implements Client.
type client struct {
	caller base.FacadeCaller
}

// Import implements Client.
func (c *client) Import(bytes []byte) error {
	serialized := params.SerializedModel{Bytes: bytes}
	return c.caller.FacadeCall("Import", serialized, nil)
}

// Abort implements Client.
func (c *client) Abort(modelUUID string) error {
	args := params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()}
	return c.caller.FacadeCall("Abort", args, nil)
}

// Activate implements Client.
func (c *client) Activate(modelUUID string) error {
	args := params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()}
	return c.caller.FacadeCall("Activate", args, nil)
}
