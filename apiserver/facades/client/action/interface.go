// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/state"
)

// State provides the subset of global state required by the
// action facade.
type State interface {
	AllApplications() ([]*state.Application, error)
	AllMachines() ([]*state.Machine, error)
	Application(name string) (*state.Application, error)
	ApplicationLeaders() (map[string]string, error)
	FindEntity(tag names.Tag) (state.Entity, error)
	GetBlockForType(t state.BlockType) (state.Block, bool, error)
	Model() (Model, error)
	WatchActionLogs(actionId string) state.StringsWatcher
}

// Model describes model state used by the action facade.
type Model interface {
	ActionByTag(tag names.ActionTag) (state.Action, error)
	AddAction(receiver state.ActionReceiver, operationID, name string, payload map[string]interface{}, parallel *bool, executionGroup *string) (state.Action, error)
	EnqueueOperation(summary string, count int) (string, error)
	FailOperationEnqueuing(operationID, failMessage string, count int) error
	FindActionsByName(name string) ([]state.Action, error)
	ListOperations(actionNames []string, actionReceivers []names.Tag, operationStatus []state.ActionStatus,
		offset, limit int,
	) ([]state.OperationInfo, bool, error)
	ModelTag() names.ModelTag
	OperationWithActions(id string) (*state.OperationInfo, error)
	Type() state.ModelType
}

type stateShim struct {
	st *state.State
}

func (s *stateShim) AllApplications() ([]*state.Application, error) {
	return s.st.AllApplications()
}

func (s *stateShim) AllMachines() ([]*state.Machine, error) {
	return s.st.AllMachines()
}

func (s *stateShim) Application(name string) (*state.Application, error) {
	return s.st.Application(name)
}

func (s *stateShim) ApplicationLeaders() (map[string]string, error) {
	return s.st.ApplicationLeaders()
}

func (s *stateShim) FindEntity(tag names.Tag) (state.Entity, error) {
	return s.st.FindEntity(tag)
}

func (s *stateShim) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	return s.st.GetBlockForType(t)
}

func (s *stateShim) Model() (Model, error) {
	return s.st.Model()
}
func (s *stateShim) WatchActionLogs(actionId string) state.StringsWatcher {
	return s.st.WatchActionLogs(actionId)
}
