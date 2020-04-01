// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// relation implements persistent local storage of a unit's relation state, and
// translation of relation changes into hooks that need to be run.
package relation

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6/hooks"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/worker/uniter/hook"
)

// State describes the state of a relation.
type State struct {
	// RelationId identifies the relation.
	// Do not use omitempty, 0 is a valid id.
	RelationId int `yaml:"id"`

	// Members is a map from unit name to the last change version
	// for which a hook.Info was delivered on the output channel.
	// keys must be in the form <application name>-<unit number>
	// to match RemoteUnits in HookInfo.
	Members map[string]int64 `yaml:"members,omitempty"`

	// ApplicationMembers is a map from application name to the last change
	// version for which a hook.Info was delivered
	ApplicationMembers map[string]int64 `yaml:"application-members,omitempty"`

	// ChangedPending indicates that a "relation-changed" hook for the given
	// unit name must be the first hook.Info to be sent to the output channel.
	ChangedPending string `yaml:"changed-pending,omitempty"`
}

// NewState returns an initial State for relationId.
func NewState(relationId int) *State {
	return &State{
		RelationId:         relationId,
		Members:            map[string]int64{},
		ApplicationMembers: map[string]int64{},
	}
}

// Validate returns an error if the supplied hook.Info does not represent
// a valid change to the relation state. Hooks must always be validated
// against the current state before they are run, to ensure that the system
// meets its guarantees about hook execution order.
func (s *State) Validate(hi hook.Info) (err error) {
	defer errors.DeferredAnnotatef(&err, "inappropriate %q for %q", hi.Kind, hi.RemoteUnit)
	if hi.RelationId != s.RelationId {
		return fmt.Errorf("expected relation %d, got relation %d", s.RelationId, hi.RelationId)
	}
	if s.Members == nil {
		return fmt.Errorf(`relation is broken and cannot be changed further`)
	}
	/// app := hi.RemoteApplication
	unit, kind := hi.RemoteUnit, hi.Kind
	// TODO(jam): 2019-10-22 I think this is the correct thing to do, but right
	//  now it breaks a lot of tests, so I want to bring it in incrementally
	/// if app == "" {
	/// 	return fmt.Errorf(`hook %v triggered for unit %q but application not set`, kind, unit)
	/// }
	if kind == hooks.RelationBroken {
		if len(s.Members) == 0 {
			return nil
		}
		return fmt.Errorf(`cannot run "relation-broken" while units still present`)
	}
	if s.ChangedPending != "" {
		// ChangedPending doesn't take an Application name, because it is
		// triggered when a unit joins so that immediately after relation-joined
		// we trigger relation-changed for the same unit.
		if unit != s.ChangedPending || kind != hooks.RelationChanged {
			return fmt.Errorf(`expected "relation-changed" for %q`, s.ChangedPending)
		}
	} else {
		/// if _, found := s.ApplicationMembers[app]; !found {
		/// 	return fmt.Errorf("unit %v hook triggered %v without corresponding app: %v", unit, kind, app)
		/// }
		if unit == "" {
			// This should be an app hook
		} else {
			if _, joined := s.Members[unit]; joined && kind == hooks.RelationJoined {
				return fmt.Errorf("unit already joined")
			} else if !joined && kind != hooks.RelationJoined {
				return fmt.Errorf("unit has not joined")
			}
		}
	}
	return nil
}

// UpdateStateForHook updates the current relation state with changes in hi.
// It must be called after the respective hook was executed successfully.
// UpdateStateForHook doesn't validate hi but guarantees that successive
// changes of the same hi are idempotent.
func (s *State) UpdateStateForHook(hi hook.Info) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to write %q hook info for %q in state", hi.Kind, hi.RemoteUnit)
	if hi.Kind == hooks.RelationBroken {
		return errors.New("broken relation, remove")
	}
	isApp := hi.RemoteUnit == ""
	// Get a copy of current state to modify, so we only update current
	// state if the new state was written successfully.
	if hi.Kind == hooks.RelationDeparted {
		// Update own state.
		if isApp {
			delete(s.ApplicationMembers, hi.RemoteApplication)
		} else {
			delete(s.Members, hi.RemoteUnit)
		}
		return nil
	}
	// Update own state.
	if isApp {
		s.ApplicationMembers[hi.RemoteApplication] = hi.ChangeVersion
	} else {
		s.Members[hi.RemoteUnit] = hi.ChangeVersion
	}
	if hi.Kind == hooks.RelationJoined {
		s.ChangedPending = hi.RemoteUnit
	} else {
		s.ChangedPending = ""
	}
	return nil
}

func (s *State) YamlString() (string, error) {
	data, err := yaml.Marshal(*s)
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(data), nil
}

// copy returns an independent copy of the state.
func (s *State) copy() *State {
	stCopy := &State{
		RelationId:     s.RelationId,
		ChangedPending: s.ChangedPending,
	}
	if s.Members != nil {
		stCopy.Members = make(map[string]int64, len(s.Members))
		for m, v := range s.Members {
			stCopy.Members[m] = v
		}
	}
	if s.ApplicationMembers != nil {
		stCopy.ApplicationMembers = make(map[string]int64, len(s.ApplicationMembers))
		for m, v := range s.ApplicationMembers {
			stCopy.ApplicationMembers[m] = v
		}
	}
	return stCopy
}
