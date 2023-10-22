// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/core/life"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

const machinerFacade = "Machiner"

// Client provides access to the Machiner API facade.
type Client struct {
	facade base.FacadeCaller
	*common.APIAddresser
}

// NewClient creates a new client-side Machiner facade.
func NewClient(caller base.APICaller, options ...Option) *Client {
	facadeCaller := base.NewFacadeCaller(caller, machinerFacade, options...)
	return &Client{
		facade:       facadeCaller,
		APIAddresser: common.NewAPIAddresser(facadeCaller),
	}
}

// machineLife requests the lifecycle of the given machine from the server.
func (c *Client) machineLife(tag names.MachineTag) (life.Value, error) {
	return common.OneLife(c.facade, tag)
}

// Machine provides access to methods of a machine through the facade.
func (c *Client) Machine(tag names.MachineTag) (*Machine, error) {
	life, err := c.machineLife(tag)
	if err != nil {
		return nil, errors.Annotate(err, "can't get life for machine")
	}
	return &Machine{
		tag:    tag,
		life:   life,
		client: c,
	}, nil
}
