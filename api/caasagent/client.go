// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/common/cloudspec"
)

// Client provides access to an agent's view of state.
type Client struct {
	facade base.FacadeCaller
	*cloudspec.CloudSpecAPI
	*common.ModelWatcher
}

// NewClient returns a version of an api client that provides functionality
// required by caas agent code.
func NewClient(caller base.APICaller) (*Client, error) {
	modelTag, isModel := caller.ModelTag()
	if !isModel {
		return nil, errors.New("expected model specific API connection")
	}
	facadeCaller := base.NewFacadeCaller(caller, "CAASAgent")
	return &Client{
		facade:       facadeCaller,
		CloudSpecAPI: cloudspec.NewCloudSpecAPI(facadeCaller, modelTag),
		ModelWatcher: common.NewModelWatcher(facadeCaller),
	}, nil
}
