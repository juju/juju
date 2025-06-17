// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/common/cloudspec"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// State provides access to an agent's view of the state.
type State struct {
	facade base.FacadeCaller
	*common.ModelWatcher
	*cloudspec.CloudSpecAPI
	*common.ControllerConfigAPI
}

// NewState returns a version of the state that provides functionality
// required by agent code.
func NewState(caller base.APICaller) (*State, error) {
	modelTag, isModel := caller.ModelTag()
	if !isModel {
		return nil, errors.New("expected model specific API connection")
	}
	facadeCaller := base.NewFacadeCaller(caller, "Agent")
	ctrlCfgApi := common.NewControllerConfig(facadeCaller)

	return &State{
		facade:              facadeCaller,
		ModelWatcher:        common.NewModelWatcher(facadeCaller),
		CloudSpecAPI:        cloudspec.NewCloudSpecAPI(facadeCaller, modelTag),
		ControllerConfigAPI: ctrlCfgApi,
	}, nil
}

func (st *State) getEntity(tag names.Tag) (*params.AgentGetEntitiesResult, error) {
	var results params.AgentGetEntitiesResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	err := st.facade.FacadeCall("GetEntities", args, &results)
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

func (st *State) StateServingInfo() (controller.StateServingInfo, error) {
	var results params.StateServingInfo
	err := st.facade.FacadeCall("StateServingInfo", nil, &results)
	if err != nil {
		return controller.StateServingInfo{}, errors.Trace(err)
	}
	return controller.StateServingInfo{
		APIPort:           results.APIPort,
		ControllerAPIPort: results.ControllerAPIPort,
		StatePort:         results.StatePort,
		Cert:              results.Cert,
		PrivateKey:        results.PrivateKey,
		CAPrivateKey:      results.CAPrivateKey,
		SharedSecret:      results.SharedSecret,
		SystemIdentity:    results.SystemIdentity,
	}, nil
}

// IsMaster reports whether the connected machine
// agent lives at the same network address as the primary
// mongo server for the replica set.
// This call will return an error if the connected
// agent is not a machine agent with model-manager
// privileges.
func (st *State) IsMaster() (bool, error) {
	var results params.IsMasterResult
	err := st.facade.FacadeCall("IsMaster", nil, &results)
	return results.Master, err
}

type Entity struct {
	st  *State
	tag names.Tag
	doc params.AgentGetEntitiesResult
}

func (st *State) Entity(tag names.Tag) (*Entity, error) {
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
			Tag:      m.tag.String(),
			Password: password,
		}},
	}
	err := m.st.facade.FacadeCall("SetPasswords", args, &results)
	if err != nil {
		return err
	}
	return results.OneError()
}

// ClearReboot clears the reboot flag of the machine.
func (m *Entity) ClearReboot() error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: m.tag.String()},
		},
	}
	err := m.st.facade.FacadeCall("ClearReboot", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// IsAllowedControllerTag returns true if the tag kind can be for a controller.
// TODO(controlleragent) - this method is needed while IAAS controllers are still machines.
func IsAllowedControllerTag(kind string) bool {
	return kind == names.ControllerAgentTagKind || kind == names.MachineTagKind
}

// IsController returns true of the tag is for a controller (machine or agent).
// TODO(controlleragent) - this method is needed while IAAS controllers are still machines.
func IsController(caller base.APICaller, tag names.Tag) (bool, error) {
	if tag.Kind() == names.ControllerAgentTagKind {
		return true, nil
	}
	apiSt, err := NewState(caller)
	if err != nil {
		return false, errors.Trace(err)
	}
	machine, err := apiSt.Entity(tag)
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
