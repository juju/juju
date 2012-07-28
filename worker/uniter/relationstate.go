package uniter

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/schema"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// RelationState is a filesystem-backed snapshot of the state of a relation.
// Concurrent modifications to the underlying state directory in any way will
// cause undefined behaviour.
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

// checker validates the format of relation unit state files.
var checker = schema.StrictFieldMap(
	schema.Fields{"change-version": schema.Int(), "changed-pending": schema.Bool()},
	schema.Defaults{"changed-pending": false},
)

// NewRelationState loads a RelationState from the subdirectory of dirpath named
// for the supplied RelationId. The directory must not contain anything other than
// valid relation unit files as written by Commit(). If the directory does not
// exist, it will be created.
func NewRelationState(dirpath string, relationId int) (*RelationState, error) {
	path := filepath.Join(dirpath, fmt.Sprintf("%d", relationId))
	rs := &RelationState{path, relationId, map[string]int{}, ""}
	walker := func(path string, fi os.FileInfo) error {
		if fi.IsDir() {
			return fmt.Errorf("directory must be flat")
		}
		name := fi.Name()
		if strings.HasSuffix(name, "~") {
			return nil
		}
		unitname, err := unitname(name)
		if err != nil {
			return err
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		m := map[string]interface{}{}
		if err := goyaml.Unmarshal(data, m); err != nil {
			return err
		} else {
			m1, err := checker.Coerce(m, nil)
			if err != nil {
				return fmt.Errorf("invalid unit file %q: %v", name, err)
			}
			m = m1.(map[string]interface{})
		}
		rs.Members[unitname] = int(m["change-version"].(int64))
		if m["changed-pending"].(bool) {
			if rs.ChangedPending != "" {
				return fmt.Errorf("%q and %q both have pending changed hooks", rs.ChangedPending, unitname)
			}
			rs.ChangedPending = unitname
		}
		return nil
	}
	if err := createWalk(path, walker); err != nil {
		return nil, fmt.Errorf("cannot load relation state from %s: %v", path, err)
	}
	return rs, nil
}

// AllRelationStates loads and returns every RelationState persisted directly
// inside the directory at the supplied path. It is an error for the directory
// to contain anything other than valid persisted RelationStates, identified by
// relation id. If the directory does not exist, it will be created.
func AllRelationStates(dirpath string) (map[int]*RelationState, error) {
	states := map[int]*RelationState{}
	walker := func(path string, fi os.FileInfo) error {
		if !fi.IsDir() {
			return fmt.Errorf("relation %q is not a directory")
		}
		relationId, err := strconv.Atoi(fi.Name())
		if err != nil {
			return fmt.Errorf("%q is not a valid relation id", fi.Name())
		}
		state, err := NewRelationState(dirpath, relationId)
		if err != nil {
			return err
		}
		states[relationId] = state
		return filepath.SkipDir
	}
	if err := createWalk(dirpath, walker); err != nil {
		return nil, err
	}
	return states, nil
}

// Validate returns an error if hi does not represent a valid change to the
// RelationState.
func (rs *RelationState) Validate(hi HookInfo) error {
	id := hi.RelationId
	if id != rs.RelationId {
		return fmt.Errorf("cannot store state for relation %d inside relation %d", id, rs.RelationId)
	}
	unit, hook, pending := hi.RemoteUnit, hi.HookKind, rs.ChangedPending
	if pending != "" {
		if unit != pending || hook != "changed" {
			return fmt.Errorf(`expected a "changed" for %q; got a %q for %q`, pending, hook, unit)
		}
	}
	if _, joined := rs.Members[unit]; joined && hook == "joined" {
		return fmt.Errorf(`invalid "joined": %q has already joined relation %d`, unit, id)
	} else if !joined && hook != "joined" {
		return fmt.Errorf("invalid %q: %q has not joined relation %d", hook, unit, id)
	}
	return nil
}

// Commit ensures the validity of; stores; and atomically writes to disk,
// the effect on the RelationState of the successful completion of the
// hook defined by the supplied HookInfo. The state is written both to
// disk and in memory such that the representations always match, and
// are both written if and only if Commit returns nil.
func (rs *RelationState) Commit(hi HookInfo) error {
	if err := rs.Validate(hi); err != nil {
		return err
	}
	name := strings.Replace(hi.RemoteUnit, "/", "-", -1)
	path := filepath.Join(rs.Path, name)
	if hi.HookKind == "departed" {
		err := os.Remove(path)
		if err == nil {
			delete(rs.Members, hi.RemoteUnit)
		}
		return err
	}
	unit := struct {
		ChangeVersion  int  `yaml:"change-version"`
		ChangedPending bool `yaml:"changed-pending,omitempty"`
	}{
		hi.ChangeVersion,
		hi.HookKind == "joined",
	}
	data, err := goyaml.Marshal(unit)
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(path+"~", data, 0600); err != nil {
		return err
	}
	err = os.Rename(path+"~", path)
	if err == nil {
		rs.Members[hi.RemoteUnit] = hi.ChangeVersion
		if hi.HookKind == "joined" {
			rs.ChangedPending = hi.RemoteUnit
		} else {
			rs.ChangedPending = ""
		}
	}
	return err
}

// createWalk walks the supplied directory tree, and calls the supplied function
// for its children, in the manner of os.Walk. If the directory does not exist,
// it is created; if the path points to anything other than a directory, an
// error is returned.
func createWalk(dirpath string, f func(path string, fi os.FileInfo) error) error {
	walker := func(path string, fi os.FileInfo, err error) error {
		if path == dirpath {
			if os.IsNotExist(err) {
				return os.Mkdir(dirpath, 0600)
			} else if !fi.IsDir() {
				return fmt.Errorf("%s must be a directory", dirpath)
			}
			return nil
		} else if err != nil {
			return err
		}
		return f(path, fi)
	}
	return filepath.Walk(dirpath, walker)
}

// unitname converts a relation unit filename to a unit name.
func unitname(filename string) (string, error) {
	i := strings.LastIndex(filename, "-")
	if i == -1 {
		return "", fmt.Errorf("invalid unit filename %q", filename)
	}
	svcName := filename[:i]
	unitId := filename[i+1:]
	if _, err := strconv.Atoi(unitId); err != nil {
		return "", fmt.Errorf("invalid unit filename %q", filename)
	}
	return svcName + "/" + unitId, nil
}
