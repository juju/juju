// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Client allows access to the CAAS operator API endpoint.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS Operator API.
func NewClient(caller base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(caller, "CAASApplication")
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
		if params.IsCodeAlreadyExists(err) {
			return nil, errors.AlreadyExists
		} else if params.IsCodeNotAssigned(err) {
			return nil, errors.NotAssigned
		}
		return nil, err
	}
	return &UnitConfig{
		UnitTag:   names.NewUnitTag(result.Result.UnitName),
		AgentConf: result.Result.AgentConf,
	}, nil
}

// UnitTermination holds the result from calling UnitTerminating.
type UnitTermination struct {
	// WillRestart is true when the unit agent should restart.
	// It will be false when the unit is dying and should shutdown normally.
	WillRestart bool
}

// UnitTerminating is to be called by the CAASUnitTerminationWorker when the uniter is
// shutting down.
func (c *Client) UnitTerminating(unit names.UnitTag) (UnitTermination, error) {
	var result params.CAASUnitTerminationResult
	args := params.Entity{
		Tag: unit.String(),
	}
	err := c.facade.FacadeCall("UnitTerminating", args, &result)
	if err != nil {
		return UnitTermination{}, err
	}
	if err := result.Error; err != nil {
		return UnitTermination{}, err
	}
	term := UnitTermination{
		WillRestart: result.WillRestart,
	}
	return term, nil
}
