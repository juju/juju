// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

// Client provides access to the undertaker API
type Client struct {
	base.ClientFacade
	st     base.APICallCloser
	facade base.FacadeCaller
}

// UndertakerClient defines the methods on the undertaker API end point.
type UndertakerClient interface {
	EnvironInfo() (params.UndertakerEnvironInfoResult, error)
	ProcessDyingEnviron() error
	RemoveEnviron() error
	WatchEnvironResources() (watcher.NotifyWatcher, error)
}

// NewClient creates a new client for accessing the undertaker API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Undertaker")
	return &Client{ClientFacade: frontend, st: st, facade: backend}
}

// EnvironInfo returns information on the environment needed by the undertaker worker.
func (c *Client) EnvironInfo() (params.UndertakerEnvironInfoResult, error) {
	result := params.UndertakerEnvironInfoResult{}
	p, err := c.params()
	if err != nil {
		return params.UndertakerEnvironInfoResult{}, errors.Trace(err)
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

// WatchEnvironResources starts a watcher for changes to the environment's
// machines and services.
func (c *Client) WatchEnvironResources() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults

	p, err := c.params()
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = c.facade.FacadeCall("WatchEnvironResources", p, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}
