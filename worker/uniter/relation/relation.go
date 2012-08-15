package relation

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/worker/uniter/hook"
	"os"
	"path/filepath"
	"strconv"
)

// State describes the state of a relation.
type State struct {
	// RelationId identifies the relation.
	RelationId int

	// Members is a map from unit name to the last change version
	// for which a hook.Info was delivered on the output channel.
	Members map[string]int

	// ChangedPending indicates that a "relation-changed" hook for the given
	// unit name must be the first hook.Info to be sent to the output channel.
	ChangedPending string
}

// clone returns an independent clone of the state.
func (s *State) clone() *State {
	clone := &State{
		RelationId:     s.RelationId,
		ChangedPending: s.ChangedPending,
	}
	if s.Members != nil {
		clone.Members = map[string]int{}
		for m, v := range s.Members {
			clone.Members[m] = v
		}
	}
	return clone
}

// Validate returns an error if the supplied hook.Info does not represent
// a valid change to the relation state. Hooks must always be validated
// against the current state before they are run, to ensure that the system
// meets its guarantees about hook execution order.
func (s *State) Validate(hi hook.Info) (err error) {
	defer errorContextf(&err, "inappropriate %q for %q", hi.Kind, hi.RemoteUnit)
	if hi.RelationId != s.RelationId {
		return fmt.Errorf("expected relation %d, got relation %d", s.RelationId, hi.RelationId)
	}
	if s.Members == nil {
		return fmt.Errorf(`relation is broken and cannot be changed further`)
	}
	unit, kind := hi.RemoteUnit, hi.Kind
	if kind == hook.RelationBroken {
		if len(s.Members) == 0 {
			return nil
		}
		return fmt.Errorf(`cannot run "relation-broken" while units still present`)
	}
	if s.ChangedPending != "" {
		if unit != s.ChangedPending || kind != hook.RelationChanged {
			return fmt.Errorf(`expected "relation-changed" for %q`, s.ChangedPending)
		}
	} else if _, joined := s.Members[unit]; joined && kind == hook.RelationJoined {
		return fmt.Errorf("unit already joined")
	} else if !joined && kind != hook.RelationJoined {
		return fmt.Errorf("unit has not joined")
	}
	return nil
}

// StateDir is a filesystem-backed representation of the state of a
// relation. Concurrent modifications to the underlying state directory
// may cause StateDir instances to exhibit undefined behaviour.
type StateDir struct {
	// path identifies the directory holding persistent state.
	path string

	// state is the cached state of the directory, which is guaranteed
	// to be synchronized with the true state so long as no concurrent
	// changes are made to the directory.
	state State
}

// State returns the current state of the relation. It will be accurate so
// long as no concurrent modifications are made to the underlying directory.
func (d *StateDir) State() *State {
	return d.state.clone()
}

// ReadStateDir loads a StateDir from the subdirectory of dirPath named
// for the supplied RelationId. Entries with names ending in "-" followed by an
// integer must be files containing valid unit data; all other names are ignored.
// If the directory does not exist, it will be created.
func ReadStateDir(dirPath string, relationId int) (d *StateDir, err error) {
	path := filepath.Join(dirPath, strconv.Itoa(relationId))
	defer errorContextf(&err, "cannot load relation state from %q", path)
	if err = ensureDir(path); err != nil {
		return nil, err
	}
	fis, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}
	state := State{
		RelationId: relationId,
		Members:    map[string]int{},
	}
	for _, fi := range fis {
		name := fi.Name()
		unitName, ok := unitName(name)
		if !ok {
			// This doesn't look like a unit file.
			continue
		}
		data, err := ioutil.ReadFile(filepath.Join(path, name))
		if err != nil {
			return nil, err
		}
		var info diskInfo
		if err = goyaml.Unmarshal(data, &info); err != nil {
			return nil, fmt.Errorf("invalid unit file %q: %v", name, err)
		}
		if info.ChangeVersion == nil {
			return nil, fmt.Errorf(`invalid unit file %q: "changed-version" not set`, name)
		}
		state.Members[unitName] = *info.ChangeVersion
		if info.ChangedPending {
			if state.ChangedPending != "" {
				return nil, fmt.Errorf("%q and %q both have pending changed hooks", state.ChangedPending, unitName)
			}
			state.ChangedPending = unitName
		}
	}
	return &StateDir{path, state}, nil
}

// ReadAllStateDirs loads and returns every StateDir persisted directly inside
// the supplied dirPath. Entries with integer names must be directories
// containing StateDir data; all other names will be ignored. If dirPath
// does not exist, it will be created.
func ReadAllStateDirs(dirPath string) (dirs map[int]*StateDir, err error) {
	defer errorContextf(&err, "cannot load relations state from %q", dirPath)
	if err = ensureDir(dirPath); err != nil {
		return nil, err
	}
	fis, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	dirs = map[int]*StateDir{}
	for _, fi := range fis {
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

// Write atomically writes to disk the relation state change embodied by
// the supplied hook.Info. It should be called after the supplied hook.Info
// has been successfully run. Write does *not* validate the supplied
// hook.Info, but *is* idempotent: that is, rewriting the most recent
// hook.Info (for example, in the course of recovery from unexpected process
// death) will not change the state either on disk or in memory.
// Attempting to write a hook.Info that is neither valid nor a repeat of the
// most recently written one will cause undefined behaviour.
func (d *StateDir) Write(hi hook.Info) (err error) {
	defer errorContextf(&err, "failed to commit %q for %q", hi.Kind, hi.RemoteUnit)
	if hi.Kind == hook.RelationBroken {
		if err = os.Remove(d.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		// If atomic delete succeeded, update own state.
		d.state.Members = nil
		return nil
	}
	name := unitFsName(hi.RemoteUnit)
	path := filepath.Join(d.path, name)
	if hi.Kind == hook.RelationDeparted {
		if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		// If atomic delete succeeded, update own state.
		delete(d.state.Members, hi.RemoteUnit)
		return nil
	}
	di := diskInfo{&hi.ChangeVersion, hi.Kind == hook.RelationJoined}
	if err := atomicWrite(path, &di); err != nil {
		return err
	}
	// If write was successful, update own state.
	d.state.Members[hi.RemoteUnit] = hi.ChangeVersion
	if hi.Kind == hook.RelationJoined {
		d.state.ChangedPending = hi.RemoteUnit
	} else {
		d.state.ChangedPending = ""
	}
	return nil
}

// diskInfo defines the relation unit data serialization.
type diskInfo struct {
	ChangeVersion  *int `yaml:"change-version"`
	ChangedPending bool `yaml:"changed-pending,omitempty"`
}
