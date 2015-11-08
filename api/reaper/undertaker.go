// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reaper

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client provides access to the reaper API
type Client struct {
	base.ClientFacade
	st     base.APICallCloser
	facade base.FacadeCaller
}

// ReaperClient defines the methods on the reaper API end point.
type ReaperClient interface {
	EnvironInfo() (params.ReaperEnvironInfoResult, error)
	ProcessDyingEnviron() error
	RemoveEnviron() error
}

// NewClient creates a new client for accessing the reaper API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Reaper")
	return &Client{ClientFacade: frontend, st: st, facade: backend}
}

// EnvironInfo returns information on the environment needed by the reaper worker.
func (c *Client) EnvironInfo() (params.ReaperEnvironInfoResult, error) {
	result := params.ReaperEnvironInfoResult{}
	p, err := c.params()
	if err != nil {
		return params.ReaperEnvironInfoResult{}, errors.Trace(err)
	}
	err = c.facade.FacadeCall("EnvironInfo", p, &result)
	return result, errors.Trace(err)
}

// ProcessDyingEnviron checks if a dying environment has any machines or services.
// If there are none, the environment's life is changed from dying to dead.
func (c *Client) ProcessDyingEnviron() error {
	p, err := c.params()
	if err != nil {
		return errors.Trace(err)
	}

	return c.facade.FacadeCall("ProcessDyingEnviron", p, nil)
}

// RemoveEnviron removes any records of this environment from Juju.
func (c *Client) RemoveEnviron() error {
	p, err := c.params()
	if err != nil {
		return errors.Trace(err)
	}
	return c.facade.FacadeCall("RemoveEnviron", p, nil)
}

func (c *Client) params() (params.Entities, error) {
	envTag, err := c.st.EnvironTag()
	if err != nil {
		return params.Entities{}, errors.Trace(err)
	}
	return params.Entities{Entities: []params.Entity{{envTag.String()}}}, nil
}
