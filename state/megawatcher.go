package state

import (
	"launchpad.net/juju-core/state/api/params"
)

// StateWatcher watches any changes to the state.
type StateWatcher struct {
	// TODO: hold the last revid that the StateWatcher saw.
}

func newStateWatcher(st *State) *StateWatcher {
	return &StateWatcher{}
}

func (w *StateWatcher) Err() error {
	return nil
}

// Stop stops the watcher.
func (w *StateWatcher) Stop() error {
	return nil
}

// Next retrieves all changes that have happened since the given revision
// number, blocking until there are some changes available.  It also
// returns the revision number of the latest change.
func (w *StateWatcher) Next() ([]params.Delta, error) {
	// This is a stub to make progress with the higher level coding.
	return []params.Delta{
		params.Delta{
			Removed: false,
			Entity: &params.ServiceInfo{
				Name:    "Example",
				Exposed: true,
			},
		},
		params.Delta{
			Removed: true,
			Entity: &params.UnitInfo{
				Name:    "MyUnit",
				Service: "Example",
			},
		},
	}, nil
}
