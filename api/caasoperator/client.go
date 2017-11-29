// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/status"
)

// Client allows access to the CAAS operator API endpoint.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS Operator API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASOperator")
	return &Client{
		facade: facadeCaller,
	}
}

// SetStatus sets the status of the specified appplication.
func (c *Client) SetStatus(
	application string,
	status status.Status,
	info string,
	data map[string]interface{},
) error {
	if !names.IsValidApplication(application) {
		return errors.NotValidf("application name %q", application)
	}
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    names.NewApplicationTag(application).String(),
			Status: status.String(),
			Info:   info,
			Data:   data,
		}},
	}
	err := c.facade.FacadeCall("SetStatus", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	return result.OneError()
}

// Charm returns information about the charm currently assigned
// to the application.
func (c *Client) Charm(application string) (_ *charm.URL, sha256 string, _ error) {
	if !names.IsValidApplication(application) {
		return nil, "", errors.NotValidf("application name %q", application)
	}
	var results params.ApplicationCharmResults
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewApplicationTag(application).String(),
		}},
	}
	err := c.facade.FacadeCall("Charm", args, &results)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	if n := len(results.Results); n != 1 {
		return nil, "", errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return nil, "", errors.Trace(err)
	}
	result := results.Results[0].Result
	curl, err := charm.ParseURL(result.URL)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	return curl, result.SHA256, nil
}
