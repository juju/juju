// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common/cloudspec"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
)

// Client provides access to an agent's view of state.
type Client struct {
	facade base.FacadeCaller
	*cloudspec.CloudSpecAPI
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
	}, nil
}

// Model returns details of the api's model.
func (st *Client) Model() (*model.Model, error) {
	var result params.Model
	err := st.facade.FacadeCall("Model", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	owner, err := names.ParseUserTag(result.OwnerTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &model.Model{
		Name:  result.Name,
		Type:  model.ModelType(result.Type),
		UUID:  result.UUID,
		Owner: owner,
	}, nil
}
