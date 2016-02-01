// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/watcher"
)

// Client provides access to the undertaker API
type Client struct {
	base.ClientFacade
	st     base.APICallCloser
	facade base.FacadeCaller
}

// UndertakerClient defines the methods on the undertaker API end point.
type UndertakerClient interface {
	ModelInfo() (params.UndertakerModelInfoResult, error)
	ProcessDyingModel() error
	RemoveModel() error
	WatchModelResources() (watcher.NotifyWatcher, error)
	ModelConfig() (*config.Config, error)
}

// NewClient creates a new client for accessing the undertaker API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Undertaker")
	return &Client{ClientFacade: frontend, st: st, facade: backend}
}

// ModelInfo returns information on the model needed by the undertaker worker.
func (c *Client) ModelInfo() (params.UndertakerModelInfoResult, error) {
	result := params.UndertakerModelInfoResult{}
	p, err := c.params()
	if err != nil {
		return params.UndertakerModelInfoResult{}, errors.Trace(err)
	}
	err = c.facade.FacadeCall("ModelInfo", p, &result)
	return result, errors.Trace(err)
}

// ProcessDyingModel checks if a dying model has any machines or services.
// If there are none, the model's life is changed from dying to dead.
func (c *Client) ProcessDyingModel() error {
	p, err := c.params()
	if err != nil {
		return errors.Trace(err)
	}

	return c.facade.FacadeCall("ProcessDyingModel", p, nil)
}

// RemoveModel removes any records of this model from Juju.
func (c *Client) RemoveModel() error {
	p, err := c.params()
	if err != nil {
		return errors.Trace(err)
	}
	return c.facade.FacadeCall("RemoveModel", p, nil)
}

func (c *Client) params() (params.Entities, error) {
	modelTag, err := c.st.ModelTag()
	if err != nil {
		return params.Entities{}, errors.Trace(err)
	}
	return params.Entities{Entities: []params.Entity{{modelTag.String()}}}, nil
}

// WatchModelResources starts a watcher for changes to the model's
// machines and services.
func (c *Client) WatchModelResources() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults

	p, err := c.params()
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = c.facade.FacadeCall("WatchModelResources", p, &results)
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
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// ModelConfig returns configuration information on the model needed
// by the undertaker worker.
func (c *Client) ModelConfig() (*config.Config, error) {
	p, err := c.params()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result params.ModelConfigResult
	err = c.facade.FacadeCall("ModelConfig", p, &result)
	if err != nil {
		return nil, err
	}
	conf, err := config.New(config.NoDefaults, result.Config)
	if err != nil {
		return nil, err
	}
	return conf, nil
}
