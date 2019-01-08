// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client provides methods that the Juju client command uses to interact
// with models stored in the Juju Server.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "ModelGeneration")
	return &Client{ClientFacade: frontend, facade: backend}
}

// AddGeneration adds a model generation to the config.
func (c *Client) AddGeneration() error {
	var result params.ErrorResult
	err := c.facade.FacadeCall("AddGeneration", nil, &result)
	if err != nil {
		return errors.Trace(err)
	}
	return result.Error
}

// CancelGeneration adds a model generation to the config.
func (c *Client) CancelGeneration() error {
	var result params.ErrorResult
	err := c.facade.FacadeCall("CancelGeneration", nil, &result)
	if err != nil {
		return errors.Trace(err)
	}
	return result.Error
}

// SwitchGeneration adds a model generation to the config.
func (c *Client) SwitchGeneration(arg string) error {
	var result params.ErrorResult
	err := c.facade.FacadeCall("SwitchGeneration", nil, &result)
	if err != nil {
		return errors.Trace(err)
	}
	return result.Error
}

// AdvanceGeneration advances a unit and/or applications to the 'next' generation.
func (c *Client) AdvanceGeneration(entities []string) error {
	var results params.ErrorResults
	var args params.Entities
	for _, entity := range entities {
		if names.IsValidApplication(entity) {
			args.Entities = append(args.Entities,
				params.Entity{names.NewApplicationTag(entity).String()})
		} else if names.IsValidUnit(entity) {
			args.Entities = append(args.Entities,
				params.Entity{names.NewUnitTag(entity).String()})
		}
	}
	err := c.facade.FacadeCall("AdvanceGeneration", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}
