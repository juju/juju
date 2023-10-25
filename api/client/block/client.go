// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client allows access to the block API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the block API.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "Block", options...)
	return &Client{ClientFacade: frontend, facade: backend}
}

// List returns blocks that are switched on for current model.
func (c *Client) List() ([]params.Block, error) {
	var blocks params.BlockResults
	if err := c.facade.FacadeCall(context.TODO(), "List", nil, &blocks); err != nil {
		return nil, errors.Trace(err)
	}

	var all []params.Block
	var allErr params.ErrorResults
	for _, result := range blocks.Results {
		if result.Error != nil {
			allErr.Results = append(allErr.Results, params.ErrorResult{result.Error})
			continue
		}
		all = append(all, result.Result)
	}
	return all, allErr.Combine()
}

// SwitchBlockOn switches desired block on for the current model.
// Valid block types are "BlockDestroy", "BlockRemove" and "BlockChange".
func (c *Client) SwitchBlockOn(blockType, msg string) error {
	args := params.BlockSwitchParams{
		Type:    blockType,
		Message: msg,
	}
	var result params.ErrorResult
	if err := c.facade.FacadeCall(context.TODO(), "SwitchBlockOn", args, &result); err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		// cope with typed error
		return errors.Trace(result.Error)
	}
	return nil
}

// SwitchBlockOff switches desired block off for the current model.
// Valid block types are "BlockDestroy", "BlockRemove" and "BlockChange".
func (c *Client) SwitchBlockOff(blockType string) error {
	args := params.BlockSwitchParams{
		Type: blockType,
	}
	var result params.ErrorResult
	if err := c.facade.FacadeCall(context.TODO(), "SwitchBlockOff", args, &result); err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		// cope with typed error
		return errors.Trace(result.Error)
	}
	return nil
}
