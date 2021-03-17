// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import "github.com/juju/errors"

// UpgradeSeriesStatus is the current status of a series upgrade for units
type UpgradeSeriesStatus string

func (s UpgradeSeriesStatus) String() string {
	return string(s)
}

const (
	UpgradeSeriesNotStarted       UpgradeSeriesStatus = "not started"
	UpgradeSeriesPrepareStarted   UpgradeSeriesStatus = "prepare started"
	UpgradeSeriesPrepareRunning   UpgradeSeriesStatus = "prepare running"
	UpgradeSeriesPrepareCompleted UpgradeSeriesStatus = "prepare completed"
	UpgradeSeriesCompleteStarted  UpgradeSeriesStatus = "complete started"
	UpgradeSeriesCompleteRunning  UpgradeSeriesStatus = "complete running"
	UpgradeSeriesCompleted        UpgradeSeriesStatus = "completed"
	UpgradeSeriesError            UpgradeSeriesStatus = "error"
)

// Graph is a type for representing a Directed acyclic graph (DAG).
type Graph map[UpgradeSeriesStatus][]UpgradeSeriesStatus

// Validate attempts to ensure that all edges from a vertex have a vertex to
// the root graph.
func (g Graph) Validate() error {
	for vertex, vertices := range g {
		for _, child := range vertices {
			if !g.ValidState(child) {
				return errors.NotValidf("vertex %q edge to vertex %q is", vertex, child)
			}
		}
	}
	return nil
}

// ValidState checks that a state is a valid vertex, as graphs have to ensure
// that all edges to other vertices are also valid then this should be fine to
// do.
func (g Graph) ValidState(state UpgradeSeriesStatus) bool {
	_, ok := g[state]
	return ok
}

// UpgradeSeriesGraph defines a graph for moving between vertices of an upgrade
// series.
func UpgradeSeriesGraph() Graph {
	return map[UpgradeSeriesStatus][]UpgradeSeriesStatus{
		UpgradeSeriesNotStarted: {
			UpgradeSeriesPrepareStarted,
			UpgradeSeriesError,
		},
		UpgradeSeriesPrepareStarted: {
			UpgradeSeriesPrepareRunning,
			UpgradeSeriesError,
		},
		UpgradeSeriesPrepareRunning: {
			UpgradeSeriesPrepareCompleted,
			UpgradeSeriesError,
		},
		UpgradeSeriesPrepareCompleted: {
			UpgradeSeriesCompleteStarted,
			UpgradeSeriesError,
		},
		UpgradeSeriesCompleteStarted: {
			UpgradeSeriesCompleteRunning,
			UpgradeSeriesError,
		},
		UpgradeSeriesCompleteRunning: {
			UpgradeSeriesCompleted,
			UpgradeSeriesError,
		},
		UpgradeSeriesCompleted: {
			UpgradeSeriesError,
		},
		UpgradeSeriesError: {},
	}
}

// UpgradeSeriesFSM defines a finite state machine from a given graph of
// possible vertices to transition. The FSM can start in any position using the
// initial state and can move along the edges to the correct vertex.
type UpgradeSeriesFSM struct {
	state    UpgradeSeriesStatus
	vertices Graph
}

// NewUpgradeSeriesFSM creates a UpgradeSeriesFSM from a graph and an initial
// state.
func NewUpgradeSeriesFSM(graph Graph, initial UpgradeSeriesStatus) (*UpgradeSeriesFSM, error) {
	if err := graph.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return &UpgradeSeriesFSM{
		state:    initial,
		vertices: graph,
	}, nil
}

// TransitionTo attempts to transition from the current state to the new given
// state. If the state is currently at the requested state, then that's
// classified as a no-op and no transition is required.
func (u *UpgradeSeriesFSM) TransitionTo(state UpgradeSeriesStatus) bool {
	if u.state == state {
		return false
	}

	for _, vertex := range u.vertices[u.state] {
		if vertex == state {
			u.state = state
			return true
		}
	}
	return false
}

// State returns the current state of the fsm.
func (u *UpgradeSeriesFSM) State() UpgradeSeriesStatus {
	return u.state
}
