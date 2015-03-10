// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.block")

// Client allows access to the block API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the block API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Block")
	logger.Debugf("\nSTORAGE FRONT-END: %#v", frontend)
	logger.Debugf("\nSTORAGE BACK-END: %#v", backend)
	return &Client{ClientFacade: frontend, facade: backend}
}

// List returns blocks that are switched on for current environment.
func (c *Client) List() ([]params.Block, error) {
	blocks := params.BlockResults{}
	if err := c.facade.FacadeCall("List", nil, &blocks); err != nil {
		return nil, errors.Trace(err)
	}

	all := []params.Block{}
	allErr := params.ErrorResults{}
	for _, result := range blocks.Results {
		if result.Error != nil {
			allErr.Results = append(allErr.Results, params.ErrorResult{result.Error})
			continue
		}
		all = append(all, result.Result)
	}
	return all, allErr.Combine()
}

// SwitchBlockOn switches desired block on for the current environment.
// Valid block types are "BlockDestroy", "BlockRemove" and "BlockChange".
func (c *Client) SwitchBlockOn(blockType, msg string) error {
	args := params.BlockSwitchParams{
		Type:    blockType,
		Message: msg,
	}
	result := params.ErrorResult{}
	if err := c.facade.FacadeCall("SwitchBlockOn", args, &result); err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// SwitchBlockOff switches desired block off for the current environment.
// Valid block types are "BlockDestroy", "BlockRemove" and "BlockChange".
func (c *Client) SwitchBlockOff(blockType string) error {
	args := params.BlockSwitchParams{
		Type: blockType,
	}
	result := params.ErrorResult{}
	if err := c.facade.FacadeCall("SwitchBlockOff", args, &result); err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}
