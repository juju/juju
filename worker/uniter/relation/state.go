// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// relation implements persistent local storage of a unit's relation state, and
// translation of relation changes into hooks that need to be run.
package relation

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/worker/uniter/hook"
)

// State describes the state of a relation.
type State struct {
	// RelationId identifies the relation.
	RelationId int

	// Members is a map from unit name to the last change version
	// for which a hook.Info was delivered on the output channel.
	Members map[string]int64

	// ApplicationMembers is a map from application name to the last change
	// version for which a hook.Info was delivered
	ApplicationMembers map[string]int64

	// ChangedPending indicates that a "relation-changed" hook for the given
	// unit name must be the first hook.Info to be sent to the output channel.
	ChangedPending string
}

// copy returns an independent copy of the state.
func (s *State) copy() *State {
	copy := &State{
		RelationId:     s.RelationId,
		ChangedPending: s.ChangedPending,
	}
	if s.Members != nil {
		copy.Members = make(map[string]int64, len(s.Members))
		for m, v := range s.Members {
			copy.Members[m] = v
		}
	}
	if s.ApplicationMembers != nil {
		copy.ApplicationMembers = make(map[string]int64, len(s.ApplicationMembers))
		for m, v := range s.ApplicationMembers {
			copy.ApplicationMembers[m] = v
		}
	}
	return copy
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

// StateDir is a filesystem-backed representation of the state of a
// relation. Concurrent modifications to the underlying state directory
// will have undefined consequences.
type StateDir struct {
	// path identifies the directory holding persistent state.
	path string

	// state is the cached state of the directory, which is guaranteed
	// to be synchronized with the true state so long as no concurrent
	// changes are made to the directory.
	state State
}

// State returns the current state of the relation.
func (d *StateDir) State() *State {
	return d.state.copy()
}

// ReadStateDir loads a StateDir from the subdirectory of dirPath named
// for the supplied RelationId. If the directory does not exist, no error
// is returned,
func ReadStateDir(dirPath string, relationId int) (d *StateDir, err error) {
	d = &StateDir{
		filepath.Join(dirPath, strconv.Itoa(relationId)),
		State{
			RelationId:         relationId,
			Members:            map[string]int64{},
			ApplicationMembers: map[string]int64{},
			ChangedPending:     "",
		},
	}
	defer errors.DeferredAnnotatef(&err, "cannot load relation state from %q", d.path)
	if _, err := os.Stat(d.path); os.IsNotExist(err) {
		return d, nil
	} else if err != nil {
		return nil, err
	}
	fis, err := ioutil.ReadDir(d.path)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		// Entries with names ending in "-" followed by an integer must be
		// files containing valid unit data; all other names are ignored.
		name := fi.Name()
		i := strings.LastIndex(name, "-")
		if i == -1 {
			continue
		}
		svcName := name[:i]
		unitId := name[i+1:]
		isApp := false
		unitOrAppName := ""
		if unitId == "app" {
			isApp = true
			unitOrAppName = svcName
		} else {
			if _, err := strconv.Atoi(unitId); err != nil {
				continue
			}
			unitOrAppName = svcName + "/" + unitId
		}
		var info diskInfo
		if err = utils.ReadYaml(filepath.Join(d.path, name), &info); err != nil {
			return nil, fmt.Errorf("invalid unit file %q: %v", name, err)
		}
		if info.ChangeVersion == nil {
			return nil, fmt.Errorf(`invalid unit file %q: "changed-version" not set`, name)
		}
		if isApp {
			d.state.ApplicationMembers[unitOrAppName] = *info.ChangeVersion
		} else {
			d.state.Members[unitOrAppName] = *info.ChangeVersion
		}
		if info.ChangedPending {
			if d.state.ChangedPending != "" {
				return nil, fmt.Errorf("%q and %q both have pending changed hooks", d.state.ChangedPending, unitOrAppName)
			}
			d.state.ChangedPending = unitOrAppName
		}
	}
	return d, nil
}

// ReadAllStateDirs loads and returns every StateDir persisted directly inside
// the supplied dirPath. If dirPath does not exist, no error is returned.
func ReadAllStateDirs(dirPath string) (dirs map[int]*StateDir, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot load relations state from %q", dirPath)
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	fis, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	dirs = map[int]*StateDir{}
	for _, fi := range fis {
		// Entries with integer names must be directories containing StateDir
		// data; all other names will be ignored.
		relationId, err := strconv.Atoi(fi.Name())
		if err != nil {
			// This doesn't look like a relation.
			continue
		}
		dir, err := ReadStateDir(dirPath, relationId)
		if err != nil {
			return nil, err
		}
		dirs[relationId] = dir
	}
	return dirs, nil
}

// Ensure creates the directory if it does not already exist.
func (d *StateDir) Ensure() error {
	return os.MkdirAll(d.path, 0755)
}

// Exists returns true if the directory for this state exists.
func (d *StateDir) Exists() bool {
	_, err := os.Stat(d.path)
	return err == nil
}

// Write atomically writes to disk the relation state change in hi.
// It must be called after the respective hook was executed successfully.
// Write doesn't validate hi but guarantees that successive writes of
// the same hi are idempotent.
func (d *StateDir) Write(hi hook.Info) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to write %q hook info for %q on state directory", hi.Kind, hi.RemoteUnit)
	if hi.Kind == hooks.RelationBroken {
		return d.Remove()
	}
	name := strings.Replace(hi.RemoteUnit, "/", "-", 1)
	isApp := false
	if hi.RemoteUnit == "" {
		isApp = true
		name = hi.RemoteApplication + "-app"
	}
	path := filepath.Join(d.path, name)
	if hi.Kind == hooks.RelationDeparted {
		if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		// If atomic delete succeeded, update own state.
		if isApp {
			delete(d.state.ApplicationMembers, hi.RemoteApplication)
		} else {
			delete(d.state.Members, hi.RemoteUnit)
		}
		return nil
	}
	di := diskInfo{&hi.ChangeVersion, hi.Kind == hooks.RelationJoined}
	if err := utils.WriteYaml(path, &di); err != nil {
		return err
	}
	// If write was successful, update own state.
	if isApp {
		d.state.ApplicationMembers[hi.RemoteApplication] = hi.ChangeVersion
	} else {
		d.state.Members[hi.RemoteUnit] = hi.ChangeVersion
	}
	if hi.Kind == hooks.RelationJoined {
		d.state.ChangedPending = hi.RemoteUnit
	} else {
		d.state.ChangedPending = ""
	}
	return nil
}

// Remove removes the directory if it exists and is empty.
func (d *StateDir) Remove() error {
	// Note(jam): 2019-10-22 os.Remove() requires the directory to be empty, but
	//  we added "foo-app" but we won't call RelationDeparted for "foo" and thus won't
	//  delete "foo-app". Instead, during relation-broken, we cleanup all related applications.
	for appMember := range d.state.ApplicationMembers {
		path := filepath.Join(d.path, appMember+"-app")
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return errors.Trace(err)
		}
	}
	if err := os.Remove(d.path); err != nil && !os.IsNotExist(err) {
		return errors.Trace(err)
	}
	// If atomic delete succeeded, update own state.
	d.state.Members = nil
	d.state.ApplicationMembers = nil
	return nil
}

// diskInfo defines the relation unit data serialization.
type diskInfo struct {
	ChangeVersion  *int64 `yaml:"change-version"`
	ChangedPending bool   `yaml:"changed-pending,omitempty"`
}
