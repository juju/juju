// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/rpc/params"
)

// Client provides access to the high availability service, used to manage controllers.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new HighAvailability client.
func NewClient(caller base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(caller, "HighAvailability")
	return &Client{ClientFacade: frontend, facade: backend}
}

// EnableHA ensures the availability of Juju controllers.
func (c *Client) EnableHA(
	numControllers int, cons constraints.Value, placement []string,
) (params.ControllersChanges, error) {

	var results params.ControllersChangeResults
	arg := params.ControllersSpecs{
		Specs: []params.ControllersSpec{{
			NumControllers: numControllers,
			Constraints:    cons,
			Placement:      placement,
		}}}

	err := c.facade.FacadeCall(context.TODO(), "EnableHA", arg, &results)
	if err != nil {
		return params.ControllersChanges{}, err
	}
	if len(results.Results) != 1 {
		return params.ControllersChanges{}, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return params.ControllersChanges{}, result.Error
	}
	return result.Result, nil
}
