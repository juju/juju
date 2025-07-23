// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
)

// PoolHelper describes methods for working with a pool-supplied state.
type PoolHelper interface {
	Release() bool
	Annotate(string)
}

// PooledState is a wrapper for a State reference, indicating that it is
// managed by a pool.
type PooledState struct {
	*State

	pool          *StatePool
	modelUUID     string
	isSystemState bool
}

var _ PoolHelper = (*PooledState)(nil)

// Release indicates that the pooled state is no longer required
// and can be removed from the pool if there are no other references
// to it.
// The return indicates whether the released state was actually removed
// from the pool - items marked for removal are only removed when released
// by all other reference holders.
func (ps *PooledState) Release() bool {
	return ps.isSystemState == false
}

// Removing returns a channel that is closed when the PooledState
// should be released by the consumer.
func (ps *PooledState) Removing() <-chan struct{} {
	return nil
}

// TODO: implement Close that hides the state.Close for a PooledState?

// Annotate writes the supplied context information back to the pool item.
// The information is stored against the unique ID for the referer,
// indicated by the itemKey member.
func (ps *PooledState) Annotate(context string) {
	// TODO...
}

// PoolItem tracks the usage of a State instance unique to a model.
// It associates context information about state usage for each reference
// holder by associating it with a unique key.
// It tracks whether the state has been marked for removal from the pool.
type PoolItem struct {
}

// StatePool is a cache of State instances for multiple
// models. Clients should call Release when they have finished with any
// state.
type StatePool struct {
	systemState *State
}

// OpenStatePool returns a new StatePool instance.
func OpenStatePool(args OpenParams) (_ *StatePool, err error) {
	logger.Tracef(context.TODO(), "opening state pool")
	pool := &StatePool{}
	st, _ := open(
		args.ControllerTag,
		args.ControllerModelTag,
		args.NewPolicy,
		args.Clock,
		args.MaxTxnAttempts,
	)
	pool.systemState = st
	return pool, nil
}

// Get returns a PooledState for a given model, creating a new State instance
// if required.
// If the State has been marked for removal, an error is returned.
func (p *StatePool) Get(modelUUID string) (*PooledState, error) {
	return &PooledState{
		State: &State{
			modelTag:           names.NewModelTag(modelUUID),
			controllerModelTag: p.systemState.controllerModelTag,
			controllerTag:      p.systemState.controllerTag,
			stateClock:         clock.WallClock,
			policy:             p.systemState.policy,
			newPolicy:          p.systemState.newPolicy,
		},
		modelUUID:     modelUUID,
		pool:          p,
		isSystemState: modelUUID == p.systemState.ModelUUID(),
	}, nil
}

// GetModel is a convenience method for getting a Model for a State.
func (p *StatePool) GetModel(modelUUID string) (*Model, PoolHelper, error) {
	st, _ := p.Get(modelUUID)
	return &Model{
		st:  st.State,
		doc: modelDoc{UUID: modelUUID},
	}, st, nil
}

// Remove takes the state out of the pool and closes it, or marks it
// for removal if it's currently being used (indicated by Gets without
// corresponding Releases). The boolean result indicates whether or
// not the state was removed.
func (p *StatePool) Remove(modelUUID string) (bool, error) {
	return true, nil
}

// SystemState returns the State passed in to NewStatePool.
func (p *StatePool) SystemState() (*State, error) {
	return p.systemState, nil
}

// Close closes all State instances in the pool.
func (p *StatePool) Close() error {
	return nil
}

// IntrospectionReport produces the output for the introspection worker
// in order to look inside the state pool.
func (p *StatePool) IntrospectionReport() string {
	return ""
}

// Report conforms to the Dependency Engine Report() interface, giving an opportunity to introspect
// what is going on at runtime.
func (p *StatePool) Report() map[string]interface{} {
	report := make(map[string]interface{})
	return report
}

// StartWorkers is used by factory.NewModel in tests.
// TODO(wallyworld) refactor to remove this dependency.
func (p *StatePool) StartWorkers(st *State) error {
	return nil
}
