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
func (c *Client) AddGeneration(modelUUID string) error {
	var result params.ErrorResult
	arg := params.Entity{Tag: names.NewModelTag(modelUUID).String()}
	err := c.facade.FacadeCall("AddGeneration", arg, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	return nil
}

// CancelGeneration cancels a model generation to the config.
func (c *Client) CancelGeneration(modelUUID string) error {
	var result params.ErrorResult
	arg := params.Entity{Tag: names.NewModelTag(modelUUID).String()}
	err := c.facade.FacadeCall("CancelGeneration", arg, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	return nil
}

// AdvanceGeneration advances a unit and/or applications to the 'next' generation.
func (c *Client) AdvanceGeneration(modelUUID string, entities []string) error {
	var results params.ErrorResults
	arg := params.AdvanceGenerationArg{Model: params.Entity{Tag: names.NewModelTag(modelUUID).String()}}
	if len(entities) == 0 {
		return errors.Trace(errors.New("No units or applications to advance"))
	}
	for _, entity := range entities {
		switch {
		case names.IsValidApplication(entity):
			arg.Entities = append(arg.Entities,
				params.Entity{Tag: names.NewApplicationTag(entity).String()})
		case names.IsValidUnit(entity):
			arg.Entities = append(arg.Entities,
				params.Entity{Tag: names.NewUnitTag(entity).String()})
		default:
			return errors.Trace(errors.New("Must be application or unit"))
		}
	}
	err := c.facade.FacadeCall("AdvanceGeneration", arg, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.Combine()
}

// HasNextGeneration returns true if the model is a "next" generation that
// has not yet been completed.
func (c *Client) HasNextGeneration(modelUUID string) (bool, error) {
	var result params.BoolResult
	arg := params.Entity{Tag: names.NewModelTag(modelUUID).String()}
	err := c.facade.FacadeCall("HasNextGeneration", arg, &result)
	if err != nil {
		return false, errors.Trace(err)
	}
	if result.Error != nil {
		return false, errors.Trace(result.Error)
	}
	return result.Result, nil
}
