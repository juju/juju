package uniter

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"path/filepath"
	"strconv"
)

// AllRelationStates loads and returns every RelationState persisted directly
// inside the directory at the supplied path. Entries with integer names must
// be directories containing RelationState data; all other names will be ignored.
// If the directory does not exist, it will be created.
func AllRelationStates(dirpath string) (states map[int]*RelationState, err error) {
	defer errorContextf(&err, "cannot load relations state from %q", dirpath)
	if err = ensureDir(dirpath); err != nil {
		return nil, err
	}
	fis, err := ioutil.ReadDir(dirpath)
	if err != nil {
		return nil, err
	}
	states = map[int]*RelationState{}
	for _, fi := range fis {
		relationId, err := strconv.Atoi(fi.Name())
		if err != nil {
			// This doesn't look like a relation.
			continue
		}
		state, err := NewRelationState(dirpath, relationId)
		if err != nil {
			return nil, err
		}
		states[relationId] = state
	}
	return states, nil
}

// RelationState is a filesystem-backed representation of the state of a
// relation. Concurrent modifications to the underlying state directory
// may cause RelationState instances to exhibit undefined behaviour.
type RelationState struct {
	// Path identifies the directory holding persistent state.
	Path string

	// RelationId identifies the relation.
	RelationId int

	// Members is a map from unit name to the last change version
	// for which a HookInfo was delivered on the output channel.
	Members map[string]int

	// ChangedPending indicates that a "changed" hook for the given unit
	// name must be the first HookInfo to be sent to the output channel.
	ChangedPending string
}

// diskInfo defines the relation unit data serialization.
type diskInfo struct {
	ChangeVersion  *int `yaml:"change-version"`
	ChangedPending bool `yaml:"changed-pending,omitempty"`
}

// NewRelationState loads a RelationState from the subdirectory of dirpath named
// for the supplied RelationId. Entries with names ending in "-" followed by an
// integer must be files containing valid unit data; all other names are ignored.
// If the directory does not exist, it will be created.
func NewRelationState(dirpath string, relationId int) (rs *RelationState, err error) {
	path := filepath.Join(dirpath, strconv.Itoa(relationId))
	defer errorContextf(&err, "cannot load relation state from %q", path)
	if err = ensureDir(path); err != nil {
		return nil, err
	}
	fis, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}
	rs = &RelationState{path, relationId, map[string]int{}, ""}
	for _, fi := range fis {
		name := fi.Name()
		unitname, ok := unitName(name)
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
		rs.Members[unitname] = *info.ChangeVersion
		if info.ChangedPending {
			if rs.ChangedPending != "" {
				return nil, fmt.Errorf("%q and %q both have pending changed hooks", rs.ChangedPending, unitname)
			}
			rs.ChangedPending = unitname
		}
	}
	return rs, nil
}

// Validate returns an error if hi does not represent a valid change to the
// RelationState. It should be called before running any relation hook, to
// ensure that the system's guarantees about hook execution order hold true.
func (rs *RelationState) Validate(hi HookInfo) (err error) {
	defer errorContextf(&err, "inappropriate %q for %q", hi.HookKind, hi.RemoteUnit)
	if hi.RelationId != rs.RelationId {
		return fmt.Errorf("expected relation %d, got relation %d", rs.RelationId, hi.RelationId)
	}
	unit, hook := hi.RemoteUnit, hi.HookKind
	if rs.ChangedPending != "" {
		if unit != rs.ChangedPending || hook != "changed" {
			return fmt.Errorf(`expected "changed" for %q`, rs.ChangedPending)
		}
	} else if _, joined := rs.Members[unit]; joined && hook == "joined" {
		return fmt.Errorf("unit already joined")
	} else if !joined && hook != "joined" {
		return fmt.Errorf("unit has not joined")
	}
	return nil
}

// Commit atomically writes to disk the relation state change embodied by
// the supplied HookInfo. It should be called after the supplied HookInfo
// has been successfully run. If the change is not valid, or cannot be
// written, an error is returned and the change is neither persisted on
// disk nor changed in memory.
func (rs *RelationState) Commit(hi HookInfo) (err error) {
	if err = rs.Validate(hi); err != nil {
		return err
	}
	defer errorContextf(&err, "failed to commit %q for %q", hi.HookKind, hi.RemoteUnit)
	name := unitFsName(hi.RemoteUnit)
	path := filepath.Join(rs.Path, name)
	if hi.HookKind == "departed" {
		if err = os.Remove(path); err != nil {
			return err
		}
		// If atomic delete succeeded, update own fields.
		delete(rs.Members, hi.RemoteUnit)
		return nil
	}
	di := diskInfo{&hi.ChangeVersion, hi.HookKind == "joined"}
	if err := atomicWrite(path, &di); err != nil {
		return err
	}
	// If write was successful, update own fields.
	rs.Members[hi.RemoteUnit] = hi.ChangeVersion
	if hi.HookKind == "joined" {
		rs.ChangedPending = hi.RemoteUnit
	} else {
		rs.ChangedPending = ""
	}
	return nil
}
