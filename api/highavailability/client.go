// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
)

var logger = loggo.GetLogger("juju.api.highavailability")

// Client provides access to the high availability service, used to manage state servers.
type Client struct {
	base.ClientFacade
	facade     base.FacadeCaller
	environTag names.EnvironTag
}

// NewClient returns a new HighAvailability client.
func NewClient(caller base.APICallCloser) *Client {
	environTag, err := caller.EnvironTag()
	if err != nil {
		logger.Errorf("ignoring invalid environment tag: %v", err)
	}
	frontend, backend := base.NewClientFacade(caller, "HighAvailability")
	return &Client{ClientFacade: frontend, facade: backend, environTag: environTag}
}

// EnsureAvailability ensures the availability of Juju state servers.
func (c *Client) EnsureAvailability(
	numStateServers int, cons constraints.Value, series string, placement []string,
) (params.StateServersChanges, error) {

	var results params.StateServersChangeResults
	arg := params.StateServersSpecs{
		Specs: []params.StateServersSpec{{
			EnvironTag:      c.environTag.String(),
			NumStateServers: numStateServers,
			Constraints:     cons,
			Series:          series,
			Placement:       placement,
		}}}

	var err error
	// We need to retain compatibility with older Juju deployments without the new HighAvailability facade.
	if c.facade.BestAPIVersion() < 1 {
		if len(placement) > 0 {
			return params.StateServersChanges{}, errors.Errorf("placement directives not supported with this version of Juju")
		}
		caller := c.facade.RawAPICaller()
		err = caller.APICall("Client", caller.BestFacadeVersion("Client"), "", "EnsureAvailability", arg, &results)
	} else {
		err = c.facade.FacadeCall("EnsureAvailability", arg, &results)
	}

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
