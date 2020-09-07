// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client allows access to the CAAS operator API endpoint.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS Operator API.
func NewClient(caller base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(caller, "Application")
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
	}
}

type UnitConfig struct {
	UnitTag   names.UnitTag
	AgentConf []byte
}

// UnitIntroduction introduces the unit and returns an agent config.
func (c *Client) UnitIntroduction(podName string, podUUID string) (*UnitConfig, error) {
	var result params.CAASUnitIntroductionResult
	args := params.CAASUnitIntroductionArgs{
		PodName: podName,
		PodUUID: podUUID,
	}
	err := c.facade.FacadeCall("UnitIntroduction", args, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	return &UnitConfig{
		UnitTag:   names.NewUnitTag(result.Result.UnitName),
		AgentConf: result.Result.AgentConf,
	}, nil
}
