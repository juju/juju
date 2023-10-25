// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

const subnetsFacade = "Subnets"

// API provides access to the Subnets API facade.
type API struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewAPI creates a new client-side Subnets facade.
func NewAPI(caller base.APICallCloser, options ...Option) *API {
	if caller == nil {
		panic("caller is nil")
	}
	clientFacade, facadeCaller := base.NewClientFacade(caller, subnetsFacade, options...)
	return &API{
		ClientFacade: clientFacade,
		facade:       facadeCaller,
	}
}

// ListSubnets fetches all the subnets known by the model.
func (api *API) ListSubnets(spaceTag *names.SpaceTag, zone string) ([]params.Subnet, error) {
	var response params.ListSubnetsResults
	var space string
	if spaceTag != nil {
		space = spaceTag.String()
	}
	args := params.SubnetsFilters{
		SpaceTag: space,
		Zone:     zone,
	}
	err := api.facade.FacadeCall(context.TODO(), "ListSubnets", args, &response)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return response.Results, nil
}

// SubnetsByCIDR returns the collection of subnets matching each CIDR in the
// input.
func (api *API) SubnetsByCIDR(cidrs []string) ([]params.SubnetsResult, error) {
	args := params.CIDRParams{CIDRS: cidrs}

	var result params.SubnetsResults
	if err := api.facade.FacadeCall(context.TODO(), "SubnetsByCIDR", args, &result); err != nil {
		if params.IsCodeNotSupported(err) {
			return nil, errors.NewNotSupported(nil, err.Error())
		}
		return nil, errors.Trace(err)
	}

	for _, result := range result.Results {
		if result.Error != nil {
			return nil, errors.Trace(result.Error)
		}
	}

	return result.Results, nil
}
