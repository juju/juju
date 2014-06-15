// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/params"
)

// State provides access to an agent's view of the state.
type State struct {
	base.FacadeCaller
}

// NewState returns a version of the state that provides functionality
// required by agent code.
func NewState(caller base.Caller) *State {
	return &State{base.GetFacadeCaller(caller, "Agent")}
}

func (st *State) getEntity(tag string) (*params.AgentGetEntitiesResult, error) {
	var results params.AgentGetEntitiesResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.APICall("GetEntities", args, &results)
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

func (st *State) StateServingInfo() (params.StateServingInfo, error) {
	var results params.StateServingInfo
	err := st.APICall("StateServingInfo", nil, &results)
	return results, err
}

// IsMaster reports whether the connected machine
// agent lives at the same network address as the primary
// mongo server for the replica set.
// This call will return an error if the connected
// agent is not a machine agent with environment-manager
// privileges.
func (st *State) IsMaster() (bool, error) {
	var results params.IsMasterResult
	err := st.APICall("IsMaster", nil, &results)
	return results.Master, err
}

type Entity struct {
	st  *State
	tag string
	doc params.AgentGetEntitiesResult
}

func (st *State) Entity(tag string) (*Entity, error) {
	doc, err := st.getEntity(tag)
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
	return m.tag
}

// Life returns the current life cycle state of the entity.
func (m *Entity) Life() params.Life {
	return m.doc.Life
}

// Jobs returns the set of configured jobs
// if the API is running on behalf of a machine agent.
// When running for other agents, it will return
// the empty list.
func (m *Entity) Jobs() []params.MachineJob {
	return m.doc.Jobs
}

// ContainerType returns the type of container hosting this entity.
// If the entity is not a machine, it returns an empty string.
func (m *Entity) ContainerType() instance.ContainerType {
	return m.doc.ContainerType
}

// SetPassword sets the password associated with the agent's entity.
func (m *Entity) SetPassword(password string) error {
	var results params.ErrorResults
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      m.tag,
			Password: password,
		}},
	}
	err := m.st.APICall("SetPasswords", args, &results)
	if err != nil {
		return err
	}
	return results.OneError()
}
