// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides access to an agent's view of the state.
type Client struct {
	facade base.FacadeCaller
	*common.ModelConfigWatcher
	*common.ControllerConfigAPI
}

// NewClient returns a version of the state that provides functionality
// required by agent code.
func NewClient(caller base.APICaller, options ...Option) (*Client, error) {
	facadeCaller := base.NewFacadeCaller(caller, "Agent", options...)
	return &Client{
		facade:              facadeCaller,
		ModelConfigWatcher:  common.NewModelConfigWatcher(facadeCaller),
		ControllerConfigAPI: common.NewControllerConfig(facadeCaller),
	}, nil
}

func (st *Client) getEntity(ctx context.Context, tag names.Tag) (*params.AgentGetEntitiesResult, error) {
	var results params.AgentGetEntitiesResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	err := st.facade.FacadeCall(ctx, "GetEntities", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Entities) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Entities))
	}
	if err := results.Entities[0].Error; err != nil {
		return nil, err
	}
	return &results.Entities[0], nil
}

func (st *Client) StateServingInfo(ctx context.Context) (controller.ControllerAgentInfo, error) {
	var results params.StateServingInfo
	err := st.facade.FacadeCall(ctx, "StateServingInfo", nil, &results)
	if err != nil {
		return controller.ControllerAgentInfo{}, errors.Trace(err)
	}
	return controller.ControllerAgentInfo{
		APIPort:        results.APIPort,
		Cert:           results.Cert,
		PrivateKey:     results.PrivateKey,
		CAPrivateKey:   results.CAPrivateKey,
		SystemIdentity: results.SystemIdentity,
	}, nil
}

type Entity struct {
	st  *Client
	tag names.Tag
	doc params.AgentGetEntitiesResult
}

func (st *Client) Entity(ctx context.Context, tag names.Tag) (*Entity, error) {
	doc, err := st.getEntity(ctx, tag)
	if err != nil {
		return nil, err
	}
	return &Entity{
		st:  st,
		tag: tag,
		doc: *doc,
	}, nil
}

// Tag returns the entity's tag.
func (m *Entity) Tag() string {
	return m.tag.String()
}

// Life returns the current life cycle state of the entity.
func (m *Entity) Life() life.Value {
	return m.doc.Life
}

// Jobs returns the set of configured jobs
// if the API is running on behalf of a machine agent.
// When running for other agents, it will return
// the empty list.
func (m *Entity) Jobs() []model.MachineJob {
	return m.doc.Jobs
}

// IsController returns true of the tag is for a controller (machine or agent).
func IsController(ctx context.Context, caller base.APICaller, tag names.Tag) (bool, error) {
	if tag.Kind() == names.ControllerAgentTagKind {
		return true, nil
	}

	apiSt, err := NewClient(caller)
	if err != nil {
		return false, errors.Trace(err)
	}
	machine, err := apiSt.Entity(ctx, tag)
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, job := range machine.Jobs() {
		if job.NeedsState() {
			return true, nil
		}
	}
	return false, nil
}
