// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
)

// Client provides access to the high availability service, used to manage state servers.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	st     *api.State
}

// NewClient returns a new HighAvailability client.
func NewClient(st *api.State) *Client {
	frontend, backend := base.NewClientFacade(st, "HighAvailability")
	return &Client{ClientFacade: frontend, facade: backend, st: st}
}

// Override for testing.
var environTag = func(c *Client) string {
	return c.st.EnvironTag()
}

// EnsureAvailability ensures the availability of Juju state servers.
func (c *Client) EnsureAvailability(
	numStateServers int, cons constraints.Value, series string, placement []*instance.Placement,
	) (params.StateServersChanges, error) {
	var results params.StateServersChangeResults
	arg := params.StateServersSpecs{
		Specs: []params.StateServersSpec{{
			EnvironTag:      environTag(c),
			NumStateServers: numStateServers,
			Constraints:     cons,
			Series:          series,
			Placement:       placement,
		}}}
	err := c.facade.FacadeCall("EnsureAvailability", arg, &results)
	if err != nil {
		return params.StateServersChanges{}, err
	}
	if len(results.Results) != 1 {
		return params.StateServersChanges{}, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return params.StateServersChanges{}, result.Error
	}
	return result.Result, nil
}
