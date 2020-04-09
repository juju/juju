// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
)

// Client provides access to controller config
type Client struct {
	facade base.FacadeCaller
	*common.ControllerConfigAPI
}

func NewClient(caller base.APICaller) (*Client, error) {
	_, isModel := caller.ModelTag()
	if !isModel {
		return nil, errors.New("expected model specific API connection")
	}
	facadeCaller := base.NewFacadeCaller(caller, "CAASAdmission")
	return &Client{
		facade:              facadeCaller,
		ControllerConfigAPI: common.NewControllerConfig(facadeCaller),
	}, nil
}
