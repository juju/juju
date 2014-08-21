// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/params"
)

type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "MetricsManager")
	return &Client{ClientFacade: frontend, facade: backend}
}

// CleanupOldMetrics calls the function of the same name on the apiserver
func (c *Client) CleanupOldMetrics() error {
	results := new(params.ErrorResult)
	err := c.facade.FacadeCall("CleanupOldMetrics", nil, results)
	if err != nil {
		return errors.Trace(err)
	}
	if results.Error != nil {
		return results.Error
	}
	return nil
}
