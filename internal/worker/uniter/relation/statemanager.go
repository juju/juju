// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"fmt"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/kr/pretty"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/rpc/params"
)

// NewStateManager creates a new StateManager instance.
func NewStateManager(rw UnitStateReadWriter, logger Logger) (StateManager, error) {
	mgr := &stateManager{unitStateRW: rw, logger: logger}
	return mgr, mgr.initialize()
}

type stateManager struct {
	unitStateRW   UnitStateReadWriter
	relationState map[int]State
	logger        Logger
	mu            sync.Mutex
}

// Relation returns a copy of the relation state for
// the given id. Returns NotFound.
func (m *stateManager) Relation(id int) (*State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.relationState[id]; ok {
		return s.copy(), nil
	}
	return nil, errors.NotFoundf("relation %d", id)
}

// RemoveRelation removes the state for the given id from the
// manager.  The change to the manager is only made when the
// data is successfully saved.
func (m *stateManager) RemoveRelation(id int, unitGetter UnitGetter, knownUnits map[string]bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.relationState[id]
	if !ok {
		return errors.NotFoundf("relation %d", id)
	}

	// Check that the member unit exists - if not we ignore it.
	// Cache the known member units so we only do any look up once per unit.
	knownMembers := set.NewStrings()
	for unitName := range st.Members {
		unitExists, ok := knownUnits[unitName]
		if !ok {
			_, err := unitGetter.Unit(names.NewUnitTag(unitName))
			if err != nil && !params.IsCodeNotFoundOrCodeUnauthorized(err) {
				return errors.Trace(err)
			}
			unitExists = err == nil
			knownUnits[unitName] = unitExists
		}
		if !unitExists {
			m.logger.Warningf("unit %v in relation %d no longer exists", unitName, id)
			continue
		}
		knownMembers.Add(unitName)
	}
	if knownMembers.Size() != 0 {
		return errors.New(fmt.Sprintf("cannot remove persisted state, relation %d has members: %v", id, knownMembers.SortedValues()))
	}
	if err := m.remove(id); err != nil {
		return err
	}
	delete(m.relationState, id)
	return nil
}

// KnownIDs returns a slice of relation ids, known to the
// state manager.
func (m *stateManager) KnownIDs() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]int, len(m.relationState))
	// 0 is a valid id, and it's the initial value of an int
	// ensure the only 0 is the slice should be there.
	i := 0
	for k := range m.relationState {
		ids[i] = k
		i += 1
	}
	return ids
}

// SetRelation persists the given state, overwriting the previous
// state for a given id or creating state at a new id. The change to
// the manager is only made when the data is successfully saved.
func (m *stateManager) SetRelation(st *State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.write(st); err != nil {
		return errors.Annotatef(err, "could not persist relation %d state", st.RelationId)
	}
	m.relationState[st.RelationId] = *st
	return nil
}

// RelationFound returns true if the state manager has a
// state for the given id.
func (m *stateManager) RelationFound(id int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.relationState[id]
	return ok
}

// initialize loads the current state into the manager.
func (m *stateManager) initialize() error {
	unitState, err := m.unitStateRW.State()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}
	m.relationState = make(map[int]State, len(unitState.RelationState))
	if m.logger.IsTraceEnabled() {
		m.logger.Tracef("initialising state manager: %# v", pretty.Formatter(unitState.RelationState))
	}
	for k, v := range unitState.RelationState {
		var state State
		if err = yaml.Unmarshal([]byte(v), &state); err != nil {
			return errors.Annotatef(err, "cannot unmarshall relation %d state", k)
		}
		m.relationState[k] = state
	}
	return nil
}

func (m *stateManager) write(st *State) error {
	newSt, err := m.stateToPersist()
	if err != nil {
		return errors.Trace(err)
	}
	str, err := st.YamlString()
	if err != nil {
		return errors.Trace(err)
	}
	newSt[st.RelationId] = str
	return m.unitStateRW.SetState(params.SetUnitStateArg{RelationState: &newSt})
}

func (m *stateManager) remove(id int) error {
	newSt, err := m.stateToPersist()
	if err != nil {
		return errors.Trace(err)
	}
	delete(newSt, id)
	return m.unitStateRW.SetState(params.SetUnitStateArg{RelationState: &newSt})
}

// stateToPersist transforms the relationState of this manager
// into a form used for UnitStateReadWriter SetState.
func (m *stateManager) stateToPersist() (map[int]string, error) {
	newSt := make(map[int]string, len(m.relationState))
	for k, v := range m.relationState {
		str, err := v.YamlString()
		if err != nil {
			return newSt, errors.Trace(err)
		}
		newSt[k] = str
	}
	return newSt, nil
}
